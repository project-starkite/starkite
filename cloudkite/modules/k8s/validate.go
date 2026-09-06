package k8s

import (
	"encoding/json"
	"fmt"
	"strings"

	"cel.dev/cel-go/cel"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
)

type celValidation struct {
	Expression string
	Message    string
	Reason     string
}

// runValidate evaluates CEL expressions or ValidatingAdmissionPolicy rules against a manifest in-process.
func runValidate(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Manifest   any    `name:"manifest"`
		Policy     any    `name:"policy"`
		Expression string `name:"expression"`
		Strict     bool   `name:"strict"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Manifest == nil {
		return nil, fmt.Errorf("k8s.validate: manifest is required")
	}

	manifestMap, err := extractMap(p.Manifest)
	if err != nil {
		return nil, fmt.Errorf("k8s.validate: manifest extraction error: %w", err)
	}

	var validations []celValidation
	if p.Expression != "" {
		validations = append(validations, celValidation{
			Expression: p.Expression,
			Message:    fmt.Sprintf("CEL validation failed: %s", p.Expression),
		})
	}

	if p.Policy != nil {
		policyMap, err := extractMap(p.Policy)
		if err != nil {
			return nil, fmt.Errorf("k8s.validate: policy extraction error: %w", err)
		}

		// Check matchConditions if present in policy spec
		if matchConds := extractListFromSpec(policyMap, "matchConditions"); len(matchConds) > 0 {
			env, err := createCELEnv()
			if err != nil {
				return nil, fmt.Errorf("k8s.validate: CEL env error: %w", err)
			}
			data := map[string]any{"object": manifestMap}
			matchedAll := true
			for _, cond := range matchConds {
				if condMap, ok := cond.(map[string]any); ok {
					if expr, ok := condMap["expression"].(string); ok && expr != "" {
						matched, err := evaluateCEL(env, expr, data)
						if err != nil || !matched {
							matchedAll = false
							break
						}
					}
				}
			}
			if !matchedAll {
				// Match conditions did not match: policy does not apply, object is valid
				return NewAttrDict(map[string]any{
					"valid":      true,
					"message":    "policy match conditions not met; validation skipped",
					"violations": []any{},
				}), nil
			}
		}

		// Extract spec.validations
		policyValidations := extractListFromSpec(policyMap, "validations")
		for _, v := range policyValidations {
			if vMap, ok := v.(map[string]any); ok {
				expr, _ := vMap["expression"].(string)
				msg, _ := vMap["message"].(string)
				reason, _ := vMap["reason"].(string)
				if expr != "" {
					if msg == "" {
						msg = fmt.Sprintf("CEL validation failed: %s", expr)
					}
					validations = append(validations, celValidation{
						Expression: expr,
						Message:    msg,
						Reason:     reason,
					})
				}
			}
		}
	}

	if len(validations) == 0 {
		return nil, fmt.Errorf("k8s.validate: either policy (with validations) or expression must be specified")
	}

	env, err := createCELEnv()
	if err != nil {
		return nil, fmt.Errorf("k8s.validate: CEL env creation error: %w", err)
	}

	evalData := map[string]any{
		"object":  manifestMap,
		"params":  map[string]any{},
		"request": map[string]any{"operation": "CREATE"},
	}

	var violations []string
	for _, v := range validations {
		pass, err := evaluateCEL(env, v.Expression, evalData)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s (eval error: %v)", v.Message, err))
		} else if !pass {
			violations = append(violations, v.Message)
		}
	}

	valid := len(violations) == 0
	msg := "validation passed"
	if !valid {
		msg = strings.Join(violations, "; ")
		if p.Strict {
			return nil, fmt.Errorf("k8s.validate: %s", msg)
		}
	}

	violationsAny := make([]any, len(violations))
	for i, viol := range violations {
		violationsAny[i] = viol
	}

	return NewAttrDict(map[string]any{
		"valid":      valid,
		"message":    msg,
		"violations": violationsAny,
	}), nil
}

func (c *K8sClient) validate(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return runValidate(thread, b, args, kwargs)
}

func createCELEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("object", cel.DynType),
		cel.Variable("oldObject", cel.DynType),
		cel.Variable("params", cel.DynType),
		cel.Variable("request", cel.DynType),
		cel.OptionalTypes(),
	)
}

func evaluateCEL(env *cel.Env, exprStr string, data map[string]any) (bool, error) {
	ast, issues := env.Compile(exprStr)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("compile %q: %w", exprStr, issues.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("program %q: %w", exprStr, err)
	}
	out, _, err := prg.Eval(data)
	if err != nil {
		return false, fmt.Errorf("eval %q: %w", exprStr, err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression %q returned non-boolean: %v", exprStr, out.Type())
	}
	return b, nil
}

func extractMap(v any) (map[string]any, error) {
	switch val := v.(type) {
	case *KubeResource:
		d := val.ToDict()
		var res map[string]any
		if err := startype.Starlark(d).Go(&res); err != nil {
			return nil, err
		}
		return res, nil
	case *AttrDict:
		return val.ToMap(), nil
	case *starlark.Dict:
		var res map[string]any
		if err := startype.Starlark(val).Go(&res); err != nil {
			return nil, err
		}
		return res, nil
	case map[string]any:
		return normalizeAnyMap(val), nil
	case map[any]any:
		return toStringMap(val), nil
	case string:
		var res map[string]any
		if err := yaml.Unmarshal([]byte(val), &res); err == nil && res != nil {
			return normalizeAnyMap(res), nil
		}
		if err := json.Unmarshal([]byte(val), &res); err != nil {
			return nil, fmt.Errorf("failed to parse string as YAML/JSON: %w", err)
		}
		return normalizeAnyMap(res), nil
	default:
		var res map[string]any
		if sv, ok := v.(starlark.Value); ok {
			if err := startype.Starlark(sv).Go(&res); err == nil && res != nil {
				return normalizeAnyMap(res), nil
			}
		}
		return nil, fmt.Errorf("unsupported type %T for manifest/policy", v)
	}
}

func toStringMap(m map[any]any) map[string]any {
	res := make(map[string]any, len(m))
	for k, v := range m {
		strK := fmt.Sprint(k)
		switch inner := v.(type) {
		case map[any]any:
			res[strK] = toStringMap(inner)
		case map[string]any:
			res[strK] = normalizeAnyMap(inner)
		case []any:
			res[strK] = toStringSlice(inner)
		default:
			res[strK] = inner
		}
	}
	return res
}

func toStringSlice(s []any) []any {
	res := make([]any, len(s))
	for i, item := range s {
		switch inner := item.(type) {
		case map[any]any:
			res[i] = toStringMap(inner)
		case map[string]any:
			res[i] = normalizeAnyMap(inner)
		case []any:
			res[i] = toStringSlice(inner)
		default:
			res[i] = inner
		}
	}
	return res
}

func normalizeAnyMap(m map[string]any) map[string]any {
	res := make(map[string]any, len(m))
	for k, v := range m {
		switch inner := v.(type) {
		case map[any]any:
			res[k] = toStringMap(inner)
		case map[string]any:
			res[k] = normalizeAnyMap(inner)
		case []any:
			res[k] = toStringSlice(inner)
		default:
			res[k] = inner
		}
	}
	return res
}

func extractListFromSpec(m map[string]any, key string) []any {
	if spec, ok := m["spec"].(map[string]any); ok {
		if list, ok := spec[key].([]any); ok {
			return list
		}
	}
	if list, ok := m[key].([]any); ok {
		return list
	}
	return nil
}

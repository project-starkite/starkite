package k8s

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func TestValidateSingleExpression(t *testing.T) {
	manifest := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name": "web",
		},
		"spec": map[string]any{
			"replicas": int64(3),
		},
	}

	thread := &starlark.Thread{Name: "test"}

	// Valid expression: replicas == 3
	val, err := runValidate(thread, nil, starlark.Tuple{}, []starlark.Tuple{
		{starlark.String("manifest"), NewAttrDict(manifest)},
		{starlark.String("expression"), starlark.String("object.spec.replicas == 3")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st, ok := val.(*starlarkstruct.Struct)
	if !ok {
		t.Fatalf("expected *starlarkstruct.Struct, got %T", val)
	}
	if validVal, err := st.Attr("valid"); err != nil || validVal != starlark.True {
		t.Fatalf("expected valid=true, got %v (err=%v)", validVal, err)
	}

	// Invalid expression: replicas > 5
	val2, err := runValidate(thread, nil, starlark.Tuple{}, []starlark.Tuple{
		{starlark.String("manifest"), NewAttrDict(manifest)},
		{starlark.String("expression"), starlark.String("object.spec.replicas > 5")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st2, ok := val2.(*starlarkstruct.Struct)
	if !ok {
		t.Fatalf("expected *starlarkstruct.Struct, got %T", val2)
	}
	if validVal2, err := st2.Attr("valid"); err != nil || validVal2 != starlark.False {
		t.Fatalf("expected valid=false, got %v (err=%v)", validVal2, err)
	}
	violVal, err := st2.Attr("violations")
	if err != nil {
		t.Fatalf("violations attr error: %v", err)
	}
	violations, ok := violVal.(*starlark.List)
	if !ok || violations.Len() != 1 {
		t.Fatalf("expected 1 violation, got %v", violVal)
	}

	// Strict=true on failure should return error
	_, err = runValidate(thread, nil, starlark.Tuple{}, []starlark.Tuple{
		{starlark.String("manifest"), NewAttrDict(manifest)},
		{starlark.String("expression"), starlark.String("object.spec.replicas > 5")},
		{starlark.String("strict"), starlark.Bool(true)},
	})
	if err == nil {
		t.Fatal("expected error with strict=true, got nil")
	}
}

func TestValidatePolicyWithOptionalTypes(t *testing.T) {
	manifest := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name": "secure-pod",
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name":  "app",
					"image": "nginx:latest",
					"securityContext": map[string]any{
						"privileged": false,
					},
				},
			},
		},
	}

	policy := map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       "ValidatingAdmissionPolicy",
		"metadata": map[string]any{
			"name": "disallow-privileged",
		},
		"spec": map[string]any{
			"validations": []any{
				map[string]any{
					"expression": "!object.spec.containers.exists(c, c.securityContext.?privileged.orValue(false))",
					"message":    "Privileged containers are not allowed",
				},
			},
		},
	}

	thread := &starlark.Thread{Name: "test"}

	val, err := runValidate(thread, nil, starlark.Tuple{}, []starlark.Tuple{
		{starlark.String("manifest"), NewAttrDict(manifest)},
		{starlark.String("policy"), NewAttrDict(policy)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := val.(*starlarkstruct.Struct)
	if validVal, err := st.Attr("valid"); err != nil || validVal != starlark.True {
		t.Fatalf("expected valid=true, got %v", validVal)
	}

	// Now make container privileged
	privManifest := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name": "bad-pod",
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name":  "app",
					"image": "nginx:latest",
					"securityContext": map[string]any{
						"privileged": true,
					},
				},
			},
		},
	}

	valPriv, err := runValidate(thread, nil, starlark.Tuple{}, []starlark.Tuple{
		{starlark.String("manifest"), NewAttrDict(privManifest)},
		{starlark.String("policy"), NewAttrDict(policy)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stPriv := valPriv.(*starlarkstruct.Struct)
	if validPriv, err := stPriv.Attr("valid"); err != nil || validPriv != starlark.False {
		t.Fatalf("expected valid=false for privileged container, got %v", validPriv)
	}
	violPrivVal, _ := stPriv.Attr("violations")
	violations := violPrivVal.(*starlark.List)
	if violations.Len() != 1 || violations.Index(0) != starlark.String("Privileged containers are not allowed") {
		t.Fatalf("unexpected violations: %v", violations)
	}
}

func TestValidateMatchConditions(t *testing.T) {
	manifest := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name": "dev-pod",
			"labels": map[string]any{
				"env": "dev",
			},
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name": "app",
					"securityContext": map[string]any{
						"privileged": true,
					},
				},
			},
		},
	}

	// Policy only applies if env == "prod"
	policy := map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       "ValidatingAdmissionPolicy",
		"spec": map[string]any{
			"matchConditions": []any{
				map[string]any{
					"name":       "is-prod",
					"expression": "object.metadata.labels.?env.orValue('') == 'prod'",
				},
			},
			"validations": []any{
				map[string]any{
					"expression": "!object.spec.containers.exists(c, c.securityContext.?privileged.orValue(false))",
					"message":    "Privileged containers are not allowed in prod",
				},
			},
		},
	}

	thread := &starlark.Thread{Name: "test"}

	// dev pod should pass because matchCondition is not met (policy skipped)
	val, err := runValidate(thread, nil, starlark.Tuple{}, []starlark.Tuple{
		{starlark.String("manifest"), NewAttrDict(manifest)},
		{starlark.String("policy"), NewAttrDict(policy)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stDev := val.(*starlarkstruct.Struct)
	if validDev, err := stDev.Attr("valid"); err != nil || validDev != starlark.True {
		t.Fatalf("expected valid=true because matchCondition skipped policy, got %v", validDev)
	}
}

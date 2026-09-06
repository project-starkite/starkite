package k8s

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// FieldType indicates the expected type for an object field.
type FieldType int

const (
	FieldString FieldType = iota
	FieldInt
	FieldBool
	FieldDict
	FieldList
	FieldKubeObject
	FieldAny
)

// FieldSpec describes a single field in a resource schema.
type FieldSpec struct {
	JSONKey    string    // camelCase output key (e.g., "replicas")
	Typ        FieldType // expected type
	Required   bool      // is this field required?
	DefaultVal any       // default value if not provided
	SpecKey    bool      // if true, field goes under "spec" in the output
	MetaKey    bool      // if true, field goes under "metadata" in template output (IsTemplate schemas)
}

// ResourceSchema defines the shape of a k8s.obj constructor.
type ResourceSchema struct {
	Kind        string                // e.g., "Deployment"
	APIVersion  string                // e.g., "apps/v1"
	IsSubObject bool                  // true for sub-objects like container, volume
	IsTemplate  bool                  // true for pod_template (renders as {metadata: {...}, spec: {...}})
	Fields      map[string]*FieldSpec // snake_case field name → spec
}

// KubeResource is a Starlark value representing a Kubernetes object built via k8s.obj.*.
type KubeResource struct {
	schema *ResourceSchema
	data   map[string]any
}

// Ensure KubeResource implements KubeObject and DictConvertible.
var _ KubeObject = (*KubeResource)(nil)
var _ startype.DictConvertible = (*KubeResource)(nil)

func (r *KubeResource) String() string {
	var parts []string
	if name, ok := r.data["name"].(string); ok {
		parts = append(parts, fmt.Sprintf("name=%q", name))
	}
	// Show a few key fields
	keys := make([]string, 0, len(r.data))
	for k := range r.data {
		if k != "name" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(parts) >= 4 {
			break
		}
		v := r.data[k]
		switch val := v.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("%s=%q", k, val))
		case int, int64, float64, bool:
			parts = append(parts, fmt.Sprintf("%s=%v", k, val))
		}
	}
	return fmt.Sprintf("<%s %s>", strings.ToLower(r.schema.Kind), strings.Join(parts, " "))
}

func (r *KubeResource) Type() string          { return "k8s.obj." + strings.ToLower(r.schema.Kind) }
func (r *KubeResource) Freeze()               {}
func (r *KubeResource) Truth() starlark.Bool  { return starlark.True }
func (r *KubeResource) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", r.Type()) }

func (r *KubeResource) Kind() string { return r.schema.Kind }

// Attr returns a field value or the to_dict method.
func (r *KubeResource) Attr(name string) (starlark.Value, error) {
	if name == "to_dict" {
		return starlark.NewBuiltin(r.Type()+".to_dict", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return r.ToDict(), nil
		}), nil
	}

	if spec, ok := r.schema.Fields[name]; ok {
		val, exists := r.data[name]
		if !exists {
			return starlark.None, nil
		}
		return goToStarlark(val, spec.Typ)
	}

	return nil, nil
}

// AttrNames returns all field names plus to_dict.
func (r *KubeResource) AttrNames() []string {
	names := make([]string, 0, len(r.schema.Fields)+1)
	for name := range r.schema.Fields {
		names = append(names, name)
	}
	names = append(names, "to_dict")
	sort.Strings(names)
	return names
}

// ToDict builds the full Kubernetes resource dict.
func (r *KubeResource) ToDict() *starlark.Dict {
	if r.schema.IsSubObject {
		return r.buildSubObjectDict()
	}
	return r.buildResourceDict()
}

func (r *KubeResource) buildResourceDict() *starlark.Dict {
	result := starlark.NewDict(4)

	result.SetKey(starlark.String("apiVersion"), starlark.String(r.schema.APIVersion))
	result.SetKey(starlark.String("kind"), starlark.String(r.schema.Kind))

	// Build metadata
	metadata := starlark.NewDict(4)
	if name, ok := r.data["name"]; ok {
		sv, _ := startype.Go(name).ToStarlarkValue()
		metadata.SetKey(starlark.String("name"), sv)
	}
	if ns, ok := r.data["namespace"]; ok {
		sv, _ := startype.Go(ns).ToStarlarkValue()
		metadata.SetKey(starlark.String("namespace"), sv)
	}
	if labels, ok := r.data["labels"]; ok {
		sv, _ := goToStarlark(labels, FieldDict)
		metadata.SetKey(starlark.String("labels"), sv)
	}
	if annotations, ok := r.data["annotations"]; ok {
		sv, _ := goToStarlark(annotations, FieldDict)
		metadata.SetKey(starlark.String("annotations"), sv)
	}
	result.SetKey(starlark.String("metadata"), metadata)

	// Build spec (non-metadata fields)
	spec := starlark.NewDict(len(r.schema.Fields))
	for fieldName, fieldSpec := range r.schema.Fields {
		if !fieldSpec.SpecKey {
			continue
		}
		val, ok := r.data[fieldName]
		if !ok {
			continue
		}
		if fieldName == "security_context" {
			if _, isKR := val.(*KubeResource); !isKR {
				val = normalizeSecurityContext(val)
			}
		}
		if fieldName == "gateway_class" || fieldName == "gateway_class_name" || fieldName == "policy_name" {
			if kr, ok := val.(*KubeResource); ok {
				if n, ok := kr.data["name"].(string); ok {
					val = n
				}
			} else if m, ok := val.(map[string]any); ok {
				if n, ok := m["name"].(string); ok {
					val = n
				} else if meta, ok := m["metadata"].(map[string]any); ok {
					if n, ok := meta["name"].(string); ok {
						val = n
					}
				}
			}
		}
		if fieldName == "parent_refs" {
			val = normalizeParentRefs(val)
		}
		switch r.schema.Kind {
		case "Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "ReferenceGrant",
			"ValidatingAdmissionPolicy", "ValidatingAdmissionPolicyBinding",
			"MutatingAdmissionPolicy", "MutatingAdmissionPolicyBinding":
			if fieldName != "parent_refs" && fieldName != "gateway_class" && fieldName != "gateway_class_name" && fieldName != "policy_name" {
				val = normalizeSnakeKeys(val)
			}
		}
		sv, err := goToStarlark(val, fieldSpec.Typ)
		if err != nil {
			continue
		}
		spec.SetKey(starlark.String(fieldSpec.JSONKey), sv)
	}
	// PVC convenience: expand "storage" shorthand into spec.resources.requests.storage
	if r.schema.Kind == "PersistentVolumeClaim" {
		if storageVal, ok := r.data["storage"]; ok {
			if _, hasResources := r.data["resources"]; !hasResources {
				requestsDict := starlark.NewDict(1)
				sv, _ := goToStarlark(storageVal, FieldString)
				requestsDict.SetKey(starlark.String("storage"), sv)
				resourcesDict := starlark.NewDict(1)
				resourcesDict.SetKey(starlark.String("requests"), requestsDict)
				spec.SetKey(starlark.String("resources"), resourcesDict)
			}
		}
	}

	// PodDisruptionBudget convenience: expand flat selector into spec.selector.matchLabels if needed
	if r.schema.Kind == "PodDisruptionBudget" {
		if selVal, ok := r.data["selector"]; ok {
			if m, ok := selVal.(map[string]any); ok {
				if _, hasML := m["matchLabels"]; !hasML {
					if _, hasME := m["matchExpressions"]; !hasME {
						matchLabelsDict := starlark.NewDict(len(m))
						for k, v := range m {
							sv, _ := startype.Go(v).ToStarlarkValue()
							_ = matchLabelsDict.SetKey(starlark.String(k), sv)
						}
						selDict := starlark.NewDict(1)
						_ = selDict.SetKey(starlark.String("matchLabels"), matchLabelsDict)
						spec.SetKey(starlark.String("selector"), selDict)
					}
				}
			}
		}
	}

	// PersistentVolume convenience: expand "storage" shorthand into spec.capacity.storage
	if r.schema.Kind == "PersistentVolume" {
		if storageVal, ok := r.data["storage"]; ok {
			if _, hasCapacity := r.data["capacity"]; !hasCapacity {
				capDict := starlark.NewDict(1)
				sv, _ := goToStarlark(storageVal, FieldString)
				capDict.SetKey(starlark.String("storage"), sv)
				spec.SetKey(starlark.String("capacity"), capDict)
			}
		}
	}

	// ResourceClaim convenience: synthesize spec.devices.requests from shortcuts
	if r.schema.Kind == "ResourceClaim" {
		if _, hasDevices := r.data["devices"]; !hasDevices {
			if devClass, ok := r.data["device_class"]; ok {
				req := starlark.NewDict(4)
				reqName := "req-1"
				if rn, ok := r.data["request_name"].(string); ok && rn != "" {
					reqName = rn
				}
				req.SetKey(starlark.String("name"), starlark.String(reqName))
				devClassVal, _ := goToStarlark(devClass, FieldString)
				req.SetKey(starlark.String("deviceClassName"), devClassVal)

				if countVal, ok := r.data["count"]; ok {
					sv, _ := goToStarlark(countVal, FieldInt)
					req.SetKey(starlark.String("count"), sv)
				}
				if allocMode, ok := r.data["allocation_mode"]; ok {
					sv, _ := goToStarlark(allocMode, FieldString)
					req.SetKey(starlark.String("allocationMode"), sv)
				}
				if selectors, ok := r.data["selectors"]; ok {
					sv, _ := goToStarlark(selectors, FieldList)
					req.SetKey(starlark.String("selectors"), sv)
				}
				if tolerations, ok := r.data["device_tolerations"]; ok {
					sv, _ := goToStarlark(tolerations, FieldList)
					req.SetKey(starlark.String("tolerations"), sv)
				}
				requestsList := starlark.NewList([]starlark.Value{req})
				devicesDict := starlark.NewDict(1)
				devicesDict.SetKey(starlark.String("requests"), requestsList)
				spec.SetKey(starlark.String("devices"), devicesDict)
			}
		}
	}

	// ResourceClaimTemplate convenience: unwrap spec.spec
	if r.schema.Kind == "ResourceClaimTemplate" {
		var claimSpec starlark.Value
		if rawSpec, ok := r.data["spec"]; ok {
			switch s := rawSpec.(type) {
			case *KubeResource:
				claimDict := s.ToDict()
				if innerSpec, found, _ := claimDict.Get(starlark.String("spec")); found {
					claimSpec = innerSpec
				} else {
					claimSpec = claimDict
				}
			case *starlark.Dict:
				if innerSpec, found, _ := s.Get(starlark.String("spec")); found {
					claimSpec = innerSpec
				} else {
					claimSpec = s
				}
			case map[string]any:
				if innerSpec, found := s["spec"]; found {
					claimSpec, _ = goToStarlarkDeep(innerSpec)
				} else {
					claimSpec, _ = goToStarlarkDeep(s)
				}
			}
		}
		if claimSpec != nil {
			spec.SetKey(starlark.String("spec"), claimSpec)
		}
		if claimMeta, ok := r.data["claim_metadata"]; ok {
			sv, _ := goToStarlark(claimMeta, FieldDict)
			spec.SetKey(starlark.String("metadata"), sv)
		}
	}

	if spec.Len() > 0 {
		result.SetKey(starlark.String("spec"), spec)
	}

	// Handle data field for ConfigMap/Secret
	if dataVal, ok := r.data["data"]; ok {
		sv, _ := goToStarlark(dataVal, FieldDict)
		result.SetKey(starlark.String("data"), sv)
	}

	// Handle remaining top-level fields (e.g. Secret.stringData, Role.rules)
	for fieldName, fieldSpec := range r.schema.Fields {
		if fieldSpec.SpecKey {
			continue // already in spec
		}
		if _, isMeta := metadataFields[fieldName]; isMeta {
			continue // already in metadata
		}
		switch fieldName {
		case "data", "storage", "device_class", "count", "allocation_mode", "request_name", "device_tolerations", "claim_metadata":
			continue // handled specially
		case "selectors":
			if r.schema.Kind == "ResourceClaim" {
				continue // shortcut for spec.devices.requests[0].selectors
			}
		}
		val, ok := r.data[fieldName]
		if !ok {
			continue
		}
		sv, err := goToStarlark(val, fieldSpec.Typ)
		if err != nil {
			continue
		}
		result.SetKey(starlark.String(fieldSpec.JSONKey), sv)
	}

	return result
}

func (r *KubeResource) buildSubObjectDict() *starlark.Dict {
	if r.schema.IsTemplate {
		return r.buildTemplateDict()
	}
	result := starlark.NewDict(len(r.schema.Fields))
	for fieldName, fieldSpec := range r.schema.Fields {
		val, ok := r.data[fieldName]
		if !ok {
			continue
		}
		// Special handling: container claims go under resources.claims
		if r.schema.Kind == "Container" && fieldName == "claims" {
			continue
		}
		// Special handling: normalize container resize_policy
		if r.schema.Kind == "Container" && fieldName == "resize_policy" {
			sv, err := goToStarlark(normalizeResizePolicy(val), fieldSpec.Typ)
			if err == nil {
				result.SetKey(starlark.String(fieldSpec.JSONKey), sv)
			}
			continue
		}
		// Special handling: normalize security_context dicts
		if fieldName == "security_context" {
			if _, isKR := val.(*KubeResource); !isKR {
				val = normalizeSecurityContext(val)
			}
		}
		// Special handling: volume shortcuts handled below
		if r.schema.Kind == "Volume" {
			switch fieldName {
			case "pvc", "claim_name", "config_map", "secret", "empty_dir", "ephemeral":
				continue
			}
		}
		sv, err := goToStarlark(val, fieldSpec.Typ)
		if err != nil {
			continue
		}
		result.SetKey(starlark.String(fieldSpec.JSONKey), sv)
	}

	// Container convenience: nest claims under resources.claims
	if r.schema.Kind == "Container" {
		if claimsVal, ok := r.data["claims"]; ok {
			claimsSV, err := goToStarlark(claimsVal, FieldList)
			if err == nil {
				resVal, found, _ := result.Get(starlark.String("resources"))
				var resDict *starlark.Dict
				if found {
					if d, ok := resVal.(*starlark.Dict); ok {
						resDict = d
					}
				}
				if resDict == nil {
					resDict = starlark.NewDict(1)
					result.SetKey(starlark.String("resources"), resDict)
				}
				resDict.SetKey(starlark.String("claims"), claimsSV)
			}
		}
	}

	// Volume convenience: expand shortcuts for pvc, claim_name, config_map, secret, empty_dir, ephemeral
	if r.schema.Kind == "Volume" {
		// Handle PVC / claim_name
		var pvcVal any
		if cn, ok := r.data["claim_name"]; ok && cn != nil {
			if s, ok := cn.(string); ok && s != "" {
				pvcVal = map[string]any{"claimName": s}
			}
		}
		if pvcRaw, ok := r.data["pvc"]; ok && pvcRaw != nil {
			switch p := pvcRaw.(type) {
			case string:
				if p != "" {
					pvcVal = map[string]any{"claimName": p}
				}
			case *KubeResource:
				if name, ok := p.data["name"].(string); ok {
					pvcVal = map[string]any{"claimName": name}
				}
			case map[string]any:
				if cn, ok := p["claim_name"]; ok {
					p["claimName"] = cn
					delete(p, "claim_name")
				}
				pvcVal = p
			case *starlark.Dict:
				m, _ := startype.Dict(p).ToMap()
				if cn, ok := m["claim_name"]; ok {
					m["claimName"] = cn
					delete(m, "claim_name")
				}
				pvcVal = m
			}
		}
		if pvcVal != nil {
			sv, _ := goToStarlark(pvcVal, FieldDict)
			result.SetKey(starlark.String("persistentVolumeClaim"), sv)
		}

		// Handle config_map
		if cmRaw, ok := r.data["config_map"]; ok && cmRaw != nil {
			var cmVal any
			switch c := cmRaw.(type) {
			case string:
				if c != "" {
					cmVal = map[string]any{"name": c}
				}
			case *KubeResource:
				if name, ok := c.data["name"].(string); ok {
					cmVal = map[string]any{"name": name}
				}
			case map[string]any:
				cmVal = c
			case *starlark.Dict:
				m, _ := startype.Dict(c).ToMap()
				cmVal = m
			}
			if cmVal != nil {
				sv, _ := goToStarlark(cmVal, FieldDict)
				result.SetKey(starlark.String("configMap"), sv)
			}
		}

		// Handle secret
		if secRaw, ok := r.data["secret"]; ok && secRaw != nil {
			var secVal any
			switch s := secRaw.(type) {
			case string:
				if s != "" {
					secVal = map[string]any{"secretName": s}
				}
			case *KubeResource:
				if name, ok := s.data["name"].(string); ok {
					secVal = map[string]any{"secretName": name}
				}
			case map[string]any:
				if sn, ok := s["name"]; ok && s["secretName"] == nil {
					s["secretName"] = sn
				}
				secVal = s
			case *starlark.Dict:
				m, _ := startype.Dict(s).ToMap()
				if sn, ok := m["name"]; ok && m["secretName"] == nil {
					m["secretName"] = sn
				}
				secVal = m
			}
			if secVal != nil {
				sv, _ := goToStarlark(secVal, FieldDict)
				result.SetKey(starlark.String("secret"), sv)
			}
		}

		// Handle empty_dir
		if edRaw, ok := r.data["empty_dir"]; ok && edRaw != nil {
			var edVal any
			switch e := edRaw.(type) {
			case bool:
				if e {
					edVal = map[string]any{}
				}
			case map[string]any:
				edVal = e
			case *starlark.Dict:
				m, _ := startype.Dict(e).ToMap()
				edVal = m
			}
			if edVal != nil {
				sv, _ := goToStarlark(edVal, FieldDict)
				result.SetKey(starlark.String("emptyDir"), sv)
			}
		}

		// Handle ephemeral (Generic Ephemeral Volume)
		if ephRaw, ok := r.data["ephemeral"]; ok && ephRaw != nil {
			var ephVal any
			switch ep := ephRaw.(type) {
			case *KubeResource:
				pvcDict := ep.ToDict()
				if innerSpec, found, _ := pvcDict.Get(starlark.String("spec")); found {
					ephVal = map[string]any{"volumeClaimTemplate": map[string]any{"spec": innerSpec}}
				} else {
					ephVal = map[string]any{"volumeClaimTemplate": map[string]any{"spec": ep.data}}
				}
			case map[string]any:
				if _, hasVct := ep["volumeClaimTemplate"]; hasVct {
					ephVal = ep
				} else if spec, hasSpec := ep["spec"]; hasSpec {
					ephVal = map[string]any{"volumeClaimTemplate": map[string]any{"spec": spec}}
				} else {
					ephVal = map[string]any{"volumeClaimTemplate": map[string]any{"spec": ep}}
				}
			case *starlark.Dict:
				m, _ := startype.Dict(ep).ToMap()
				if _, hasVct := m["volumeClaimTemplate"]; hasVct {
					ephVal = m
				} else if spec, hasSpec := m["spec"]; hasSpec {
					ephVal = map[string]any{"volumeClaimTemplate": map[string]any{"spec": spec}}
				} else {
					ephVal = map[string]any{"volumeClaimTemplate": map[string]any{"spec": m}}
				}
			}
			if ephVal != nil {
				sv, _ := goToStarlark(ephVal, FieldDict)
				result.SetKey(starlark.String("ephemeral"), sv)
			}
		}
	}

	return result
}

// buildTemplateDict renders an IsTemplate sub-object as {metadata: {...}, spec: {...}}.
func (r *KubeResource) buildTemplateDict() *starlark.Dict {
	result := starlark.NewDict(2)
	metadata := starlark.NewDict(2)
	spec := starlark.NewDict(len(r.schema.Fields))

	for fieldName, fieldSpec := range r.schema.Fields {
		val, ok := r.data[fieldName]
		if !ok {
			continue
		}
		sv, err := goToStarlark(val, fieldSpec.Typ)
		if err != nil {
			continue
		}
		if fieldSpec.MetaKey {
			metadata.SetKey(starlark.String(fieldSpec.JSONKey), sv)
		} else {
			spec.SetKey(starlark.String(fieldSpec.JSONKey), sv)
		}
	}

	if metadata.Len() > 0 {
		result.SetKey(starlark.String("metadata"), metadata)
	}
	if spec.Len() > 0 {
		result.SetKey(starlark.String("spec"), spec)
	}
	return result
}

// workloadKinds lists resource kinds that support pod field flattening.
var workloadKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
	"Job":         true,
	"CronJob":     true,
}

// podFieldNames are the snake_case field names consumed by autoTemplate.
var podFieldNames = []string{
	"containers", "init_containers", "volumes", "restart_policy",
	"node_selector", "tolerations", "affinity", "service_account",
	"host_network", "dns_policy", "security_context", "resource_claims",
	"host_users",
}

// normalizeResizePolicy converts user-friendly snake_case keys in container resize_policy
// (resource_name, restart_policy) to Kubernetes camelCase JSONKeys (resourceName, restartPolicy).
func normalizeResizePolicy(val any) any {
	items, ok := val.([]any)
	if !ok {
		return val
	}
	normalized := make([]any, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			normalized[i] = item
			continue
		}
		entry := make(map[string]any, len(m))
		for k, v := range m {
			switch k {
			case "resource_name":
				entry["resourceName"] = v
			case "restart_policy":
				entry["restartPolicy"] = v
			default:
				entry[k] = v
			}
		}
		normalized[i] = entry
	}
	return normalized
}

// normalizeSecurityContext converts user-friendly snake_case keys in security_context dicts
// to Kubernetes camelCase JSONKeys.
func normalizeSecurityContext(val any) any {
	m, ok := val.(map[string]any)
	if !ok {
		return val
	}
	entry := make(map[string]any, len(m))
	for k, v := range m {
		switch k {
		case "apparmor_profile":
			entry["appArmorProfile"] = v
		case "seccomp_profile":
			entry["seccompProfile"] = v
		case "run_as_non_root":
			entry["runAsNonRoot"] = v
		case "read_only_root", "read_only_root_filesystem":
			entry["readOnlyRootFilesystem"] = v
		case "run_as_user":
			entry["runAsUser"] = v
		case "run_as_group":
			entry["runAsGroup"] = v
		case "windows_options":
			entry["windowsOptions"] = v
		case "fs_group":
			entry["fsGroup"] = v
		case "fs_group_change_policy":
			entry["fsGroupChangePolicy"] = v
		default:
			entry[k] = v
		}
	}
	return entry
}

// normalizeResourceClaims converts user-friendly snake_case keys in resource claim
// references (e.g. claim_name, template_name) to Kubernetes camelCase JSONKeys
// (resourceClaimName, resourceClaimTemplateName).
func normalizeResourceClaims(val any) any {
	items, ok := val.([]any)
	if !ok {
		return val
	}
	normalized := make([]any, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			normalized[i] = item
			continue
		}
		entry := make(map[string]any, len(m))
		for k, v := range m {
			switch k {
			case "claim_name", "resource_claim_name":
				entry["resourceClaimName"] = v
			case "template_name", "resource_claim_template_name":
				entry["resourceClaimTemplateName"] = v
			default:
				entry[k] = v
			}
		}
		normalized[i] = entry
	}
	return normalized
}

// snakeToCamel converts a snake_case identifier to lowerCamelCase.
func snakeToCamel(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}
	var b strings.Builder
	capitalizeNext := false
	for _, r := range s {
		if r == '_' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			b.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizeSnakeKeys recursively converts dictionary keys in nested maps/lists
// from snake_case to Kubernetes camelCase.
func normalizeSnakeKeys(val any) any {
	switch v := val.(type) {
	case *starlark.Dict:
		d := starlark.NewDict(v.Len())
		for _, item := range v.Items() {
			k, ok := starlark.AsString(item[0])
			if ok {
				d.SetKey(starlark.String(snakeToCamel(k)), normalizeSnakeKeysValue(item[1]))
			} else {
				d.SetKey(item[0], normalizeSnakeKeysValue(item[1]))
			}
		}
		return d
	case *starlark.List:
		elems := make([]starlark.Value, v.Len())
		for i := 0; i < v.Len(); i++ {
			elems[i] = normalizeSnakeKeysValue(v.Index(i))
		}
		return starlark.NewList(elems)
	case map[string]any:
		res := make(map[string]any, len(v))
		for k, item := range v {
			res[snakeToCamel(k)] = normalizeSnakeKeys(item)
		}
		return res
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			res[i] = normalizeSnakeKeys(item)
		}
		return res
	default:
		return val
	}
}

func normalizeSnakeKeysValue(val starlark.Value) starlark.Value {
	switch v := val.(type) {
	case *starlark.Dict:
		d := starlark.NewDict(v.Len())
		for _, item := range v.Items() {
			k, ok := starlark.AsString(item[0])
			if ok {
				d.SetKey(starlark.String(snakeToCamel(k)), normalizeSnakeKeysValue(item[1]))
			} else {
				d.SetKey(item[0], normalizeSnakeKeysValue(item[1]))
			}
		}
		return d
	case *starlark.List:
		elems := make([]starlark.Value, v.Len())
		for i := 0; i < v.Len(); i++ {
			elems[i] = normalizeSnakeKeysValue(v.Index(i))
		}
		return starlark.NewList(elems)
	default:
		return val
	}
}

// normalizeParentRefs converts parent references (strings, KubeResource, or dicts)
// to standard Gateway API parentRef structures.
func normalizeParentRefs(val any) any {
	switch v := val.(type) {
	case *starlark.List:
		elems := make([]starlark.Value, v.Len())
		for i := 0; i < v.Len(); i++ {
			item := v.Index(i)
			switch it := item.(type) {
			case *KubeResource:
				d := starlark.NewDict(1)
				if name, ok := it.data["name"].(string); ok {
					d.SetKey(starlark.String("name"), starlark.String(name))
				}
				elems[i] = d
			case starlark.String:
				d := starlark.NewDict(1)
				d.SetKey(starlark.String("name"), it)
				elems[i] = d
			default:
				elems[i] = normalizeSnakeKeysValue(it)
			}
		}
		return starlark.NewList(elems)
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			switch it := item.(type) {
			case *KubeResource:
				name, _ := it.data["name"].(string)
				res[i] = map[string]any{"name": name}
			case string:
				res[i] = map[string]any{"name": it}
			default:
				res[i] = normalizeSnakeKeys(it)
			}
		}
		return res
	default:
		return normalizeSnakeKeys(val)
	}
}

// autoTemplate builds spec.template (and spec.selector) automatically when
// `containers` is provided to a workload constructor without an explicit `template`.
// For CronJob, it builds the full jobTemplate.spec.template chain.
func autoTemplate(schema *ResourceSchema, data map[string]any) error {
	if !workloadKinds[schema.Kind] {
		return nil
	}

	_, hasContainers := data["containers"]
	_, hasTemplate := data["template"]
	_, hasJobTemplate := data["job_template"]

	if !hasContainers {
		return nil // no auto-build needed
	}

	if hasTemplate {
		return fmt.Errorf("k8s.obj.%s: cannot specify both 'containers' and 'template'", strings.ToLower(schema.Kind))
	}
	if hasJobTemplate {
		return fmt.Errorf("k8s.obj.%s: cannot specify both 'containers' and 'job_template'", strings.ToLower(schema.Kind))
	}

	// Build pod spec from pod-level fields
	podSpec := map[string]any{}
	for _, field := range podFieldNames {
		if v, ok := data[field]; ok {
			spec := podTemplateFields[field]
			if field == "resource_claims" {
				podSpec[spec.JSONKey] = normalizeResourceClaims(v)
			} else if field == "security_context" {
				if _, isKR := v.(*KubeResource); !isKR {
					podSpec[spec.JSONKey] = normalizeSecurityContext(v)
				} else {
					podSpec[spec.JSONKey] = v
				}
			} else {
				podSpec[spec.JSONKey] = v
			}
			delete(data, field)
		}
	}

	// Build template metadata
	tmplLabels := data["labels"] // inherit from resource labels by default
	if tl, ok := data["template_labels"]; ok {
		tmplLabels = tl
		delete(data, "template_labels")
	}
	tmplMeta := map[string]any{}
	if tmplLabels != nil {
		tmplMeta["labels"] = tmplLabels
	}
	if ta, ok := data["template_annotations"]; ok {
		tmplMeta["annotations"] = ta
		delete(data, "template_annotations")
	}

	template := map[string]any{
		"metadata": tmplMeta,
		"spec":     podSpec,
	}

	if schema.Kind == "CronJob" {
		// CronJob: build jobTemplate.spec.template chain
		data["job_template"] = map[string]any{
			"spec": map[string]any{
				"template": template,
			},
		}
	} else {
		data["template"] = template
	}

	// Auto-derive selector if not provided
	if _, hasSelector := data["selector"]; !hasSelector && tmplLabels != nil {
		data["selector"] = map[string]any{"matchLabels": tmplLabels}
	}

	return nil
}

// newKubeResource constructs a KubeResource from Starlark args/kwargs.
// Supports dual-mode: positional string (YAML) or dict, plus kwargs for overrides.
func newKubeResource(schema *ResourceSchema, args starlark.Tuple, kwargs []starlark.Tuple) (*KubeResource, error) {
	data := make(map[string]any)

	// Handle positional argument: YAML string or dict
	if len(args) > 0 {
		if len(args) > 1 {
			return nil, fmt.Errorf("k8s.obj.%s: expected at most 1 positional argument, got %d", strings.ToLower(schema.Kind), len(args))
		}
		switch v := args[0].(type) {
		case starlark.String:
			parsed, err := parseYAML(string(v))
			if err != nil {
				return nil, fmt.Errorf("k8s.obj.%s: %w", strings.ToLower(schema.Kind), err)
			}
			m, err := startype.Dict(parsed).ToMap()
			if err != nil {
				return nil, fmt.Errorf("k8s.obj.%s: %w", strings.ToLower(schema.Kind), err)
			}
			extractFieldsFromMap(schema, m, data)
		case *starlark.Dict:
			m, err := startype.Dict(v).ToMap()
			if err != nil {
				return nil, fmt.Errorf("k8s.obj.%s: %w", strings.ToLower(schema.Kind), err)
			}
			extractFieldsFromMap(schema, m, data)
		default:
			return nil, fmt.Errorf("k8s.obj.%s: positional arg must be YAML string or dict, got %s", strings.ToLower(schema.Kind), v.Type())
		}
	}

	// Apply kwargs (override positional data)
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]

		spec, ok := schema.Fields[key]
		if !ok {
			return nil, fmt.Errorf("k8s.obj.%s: unknown field %q", strings.ToLower(schema.Kind), key)
		}

		goVal, err := starlarkToGo(val, spec.Typ)
		if err != nil {
			return nil, fmt.Errorf("k8s.obj.%s: field %q: %w", strings.ToLower(schema.Kind), key, err)
		}
		if key == "resource_claims" {
			goVal = normalizeResourceClaims(goVal)
		}
		if key == "gateway_class" || key == "gateway_class_name" || key == "policy_name" {
			if kr, ok := val.(*KubeResource); ok {
				if n, ok := kr.data["name"].(string); ok {
					goVal = n
				}
			}
		}
		data[key] = goVal
	}

	// Auto-build template for workload types when containers= is provided
	if err := autoTemplate(schema, data); err != nil {
		return nil, err
	}

	// Check required fields
	for fieldName, spec := range schema.Fields {
		if spec.Required {
			if _, ok := data[fieldName]; !ok {
				return nil, fmt.Errorf("k8s.obj.%s: missing required field %q", strings.ToLower(schema.Kind), fieldName)
			}
		}
	}

	// Apply defaults
	for fieldName, spec := range schema.Fields {
		if spec.DefaultVal != nil {
			if _, ok := data[fieldName]; !ok {
				data[fieldName] = spec.DefaultVal
			}
		}
	}

	return &KubeResource{schema: schema, data: data}, nil
}

// extractFieldsFromMap populates data from a raw map (from YAML or dict).
func extractFieldsFromMap(schema *ResourceSchema, m map[string]any, data map[string]any) {
	if schema.IsSubObject {
		// Sub-objects: fields are at top level
		for fieldName, spec := range schema.Fields {
			if v, ok := m[spec.JSONKey]; ok {
				if fieldName == "resource_claims" {
					v = normalizeResourceClaims(v)
				}
				data[fieldName] = v
			}
		}
		if schema.Kind == "Container" {
			if res, ok := m["resources"].(map[string]any); ok {
				if cl, ok := res["claims"]; ok {
					if _, hasClaims := data["claims"]; !hasClaims {
						data["claims"] = cl
					}
				}
			}
		}
		return
	}

	// Top-level resources: extract from metadata and spec
	if metadata, ok := m["metadata"].(map[string]any); ok {
		if name, ok := metadata["name"]; ok {
			data["name"] = name
		}
		if ns, ok := metadata["namespace"]; ok {
			data["namespace"] = ns
		}
		if labels, ok := metadata["labels"]; ok {
			data["labels"] = labels
		}
		if annotations, ok := metadata["annotations"]; ok {
			data["annotations"] = annotations
		}
	}

	if spec, ok := m["spec"].(map[string]any); ok {
		for fieldName, fieldSpec := range schema.Fields {
			if !fieldSpec.SpecKey {
				continue
			}
			if v, ok := spec[fieldSpec.JSONKey]; ok {
				if fieldName == "resource_claims" {
					v = normalizeResourceClaims(v)
				}
				data[fieldName] = v
			}
		}
	}

	// Direct top-level fields (e.g., "data" for ConfigMap)
	if dataVal, ok := m["data"]; ok {
		data["data"] = dataVal
	}
}

// goToStarlark converts a Go value to a starlark.Value with type hint.
func goToStarlark(val any, typ FieldType) (starlark.Value, error) {
	if val == nil {
		return starlark.None, nil
	}

	switch typ {
	case FieldKubeObject:
		// If it's a KubeResource, return its dict
		if kr, ok := val.(*KubeResource); ok {
			return kr.ToDict(), nil
		}
		// If it's already a starlark dict, pass through
		if d, ok := val.(*starlark.Dict); ok {
			return d, nil
		}
		// If it's a map, convert (use goToStarlarkDeep to resolve nested KubeResource)
		if m, ok := val.(map[string]any); ok {
			return goToStarlarkDeep(m)
		}
	case FieldList:
		if items, ok := val.([]any); ok {
			elems := make([]starlark.Value, 0, len(items))
			for _, item := range items {
				// Try KubeResource first
				if kr, ok := item.(*KubeResource); ok {
					elems = append(elems, kr.ToDict())
				} else {
					sv, err := startype.Go(item).ToStarlarkValue()
					if err != nil {
						return nil, err
					}
					elems = append(elems, sv)
				}
			}
			return starlark.NewList(elems), nil
		}
	case FieldDict:
		if m, ok := val.(map[string]any); ok {
			return goToStarlarkDeep(m)
		}
	case FieldAny:
		if kr, ok := val.(*KubeResource); ok {
			return kr.ToDict(), nil
		}
		if m, ok := val.(map[string]any); ok {
			return goToStarlarkDeep(m)
		}
	}

	return startype.Go(val).ToStarlarkValue()
}

// resolveStarlarkValue recursively walks a Starlark value and replaces
// KubeResource with ToDict() so startype sees only standard types.
func resolveStarlarkValue(v starlark.Value) starlark.Value {
	switch val := v.(type) {
	case *KubeResource:
		return val.ToDict()
	case *starlark.Dict:
		resolved := starlark.NewDict(val.Len())
		for _, kv := range val.Items() {
			resolved.SetKey(kv[0], resolveStarlarkValue(kv[1]))
		}
		return resolved
	case *starlark.List:
		elems := make([]starlark.Value, val.Len())
		for i := 0; i < val.Len(); i++ {
			elems[i] = resolveStarlarkValue(val.Index(i))
		}
		return starlark.NewList(elems)
	default:
		return v
	}
}

// goToStarlarkDeep converts a Go value to a starlark.Value, handling
// nested *KubeResource in maps and slices (used by FieldDict).
func goToStarlarkDeep(val any) (starlark.Value, error) {
	switch v := val.(type) {
	case *KubeResource:
		return v.ToDict(), nil
	case map[string]any:
		d := starlark.NewDict(len(v))
		for k, item := range v {
			sv, err := goToStarlarkDeep(item)
			if err != nil {
				return nil, err
			}
			d.SetKey(starlark.String(k), sv)
		}
		return d, nil
	case []any:
		elems := make([]starlark.Value, len(v))
		for i, item := range v {
			sv, err := goToStarlarkDeep(item)
			if err != nil {
				return nil, err
			}
			elems[i] = sv
		}
		return starlark.NewList(elems), nil
	default:
		return startype.Go(val).ToStarlarkValue()
	}
}

// starlarkToGo converts a starlark.Value to a Go value with type hint.
func starlarkToGo(val starlark.Value, typ FieldType) (any, error) {
	if kr, ok := val.(*KubeResource); ok {
		if typ == FieldString {
			if n, ok := kr.data["name"].(string); ok {
				return n, nil
			}
		}
	}
	switch typ {
	case FieldKubeObject:
		if kr, ok := val.(*KubeResource); ok {
			return kr, nil
		}
		if d, ok := val.(*starlark.Dict); ok {
			return startype.Dict(d).ToMap()
		}
		return nil, fmt.Errorf("expected k8s object or dict, got %s", val.Type())

	case FieldList:
		if l, ok := val.(*starlark.List); ok {
			items := make([]any, 0, l.Len())
			for i := 0; i < l.Len(); i++ {
				item := l.Index(i)
				if kr, ok := item.(*KubeResource); ok {
					items = append(items, kr)
				} else {
					goVal, err := startype.Starlark(item).ToGoValue()
					if err != nil {
						return nil, err
					}
					items = append(items, goVal)
				}
			}
			return items, nil
		}
		return nil, fmt.Errorf("expected list, got %s", val.Type())

	case FieldDict:
		if d, ok := val.(*starlark.Dict); ok {
			resolved := resolveStarlarkValue(d).(*starlark.Dict)
			return startype.Dict(resolved).ToMap()
		}
		return nil, fmt.Errorf("expected dict, got %s", val.Type())

	case FieldAny:
		if kr, ok := val.(*KubeResource); ok {
			return kr, nil
		}
		if d, ok := val.(*starlark.Dict); ok {
			resolved := resolveStarlarkValue(d).(*starlark.Dict)
			return startype.Dict(resolved).ToMap()
		}
		return startype.Starlark(val).ToGoValue()

	default:
		return startype.Starlark(val).ToGoValue()
	}
}

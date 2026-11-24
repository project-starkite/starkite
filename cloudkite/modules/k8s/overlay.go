package k8s

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	yamllib "github.com/project-starkite/starkite/libkite/modules/yaml"
	"go.starlark.net/starlark"
	goyaml "gopkg.in/yaml.v3"
)

var clusterScopedKinds = map[string]bool{
	"Namespace":                        true,
	"Node":                             true,
	"ClusterRole":                      true,
	"ClusterRoleBinding":               true,
	"CustomResourceDefinition":         true,
	"PersistentVolume":                 true,
	"StorageClass":                     true,
	"PriorityClass":                    true,
	"ValidatingAdmissionPolicy":        true,
	"ValidatingAdmissionPolicyBinding": true,
	"MutatingAdmissionPolicy":          true,
	"MutatingAdmissionPolicyBinding":   true,
	"ValidatingWebhookConfiguration":   true,
	"MutatingWebhookConfiguration":     true,
	"GatewayClass":                     true,
	"VolumeAttributesClass":            true,
	"CSIDriver":                        true,
	"RuntimeClass":                     true,
	"IngressClass":                     true,
	"ClusterTrustBundle":               true,
	"CertificateSigningRequest":        true,
	"FlowSchema":                       true,
	"PriorityLevelConfiguration":       true,
}

type overlayImageSpec struct {
	Direct  string
	NewName string
	NewTag  string
	Digest  string
}

// runOverlay applies offline Kubernetes-aware transforms to a list of manifests.
// Signature: k8s.overlay(resources, namespace="", name_prefix="", name_suffix="",
// labels={}, annotations={}, images={}, patches=[], extra=[]) -> list[dict]
func runOverlay(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		resourcesVal   starlark.Value
		extraVal       starlark.Value
		namespace      string
		namePrefix     string
		nameSuffix     string
		labelsMap      map[string]any
		annotationsMap map[string]any
		imageSpecs     map[string]overlayImageSpec
		patchesList    []starlark.Value
	)

	if len(args) > 1 {
		return nil, fmt.Errorf("k8s.overlay: expected at most 1 positional argument (resources), got %d", len(args))
	}
	if len(args) == 1 {
		resourcesVal = args[0]
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "resources":
			if resourcesVal != nil {
				return nil, fmt.Errorf("k8s.overlay: resources provided both positionally and as keyword")
			}
			resourcesVal = val
		case "namespace":
			s, ok := starlark.AsString(val)
			if !ok {
				return nil, fmt.Errorf("k8s.overlay: namespace must be a string, got %s", val.Type())
			}
			namespace = s
		case "name_prefix":
			s, ok := starlark.AsString(val)
			if !ok {
				return nil, fmt.Errorf("k8s.overlay: name_prefix must be a string, got %s", val.Type())
			}
			namePrefix = s
		case "name_suffix":
			s, ok := starlark.AsString(val)
			if !ok {
				return nil, fmt.Errorf("k8s.overlay: name_suffix must be a string, got %s", val.Type())
			}
			nameSuffix = s
		case "labels":
			m, err := extractNormalizedMap(val)
			if err != nil {
				return nil, fmt.Errorf("k8s.overlay: labels: %w", err)
			}
			labelsMap = m
		case "annotations":
			m, err := extractNormalizedMap(val)
			if err != nil {
				return nil, fmt.Errorf("k8s.overlay: annotations: %w", err)
			}
			annotationsMap = m
		case "images":
			specs, err := parseImageSpecs(val)
			if err != nil {
				return nil, fmt.Errorf("k8s.overlay: images: %w", err)
			}
			imageSpecs = specs
		case "patches":
			items, err := starlarkSequence(val, "patches")
			if err != nil {
				return nil, fmt.Errorf("k8s.overlay: %w", err)
			}
			patchesList = items
		case "extra":
			extraVal = val
		default:
			return nil, fmt.Errorf("k8s.overlay: unexpected keyword argument %q", key)
		}
	}

	if resourcesVal == nil {
		return nil, fmt.Errorf("k8s.overlay: missing required argument: resources")
	}

	docs, err := normalizeOverlayInputs(resourcesVal, "resources")
	if err != nil {
		return nil, fmt.Errorf("k8s.overlay: %w", err)
	}
	if extraVal != nil {
		extraDocs, err := normalizeOverlayInputs(extraVal, "extra")
		if err != nil {
			return nil, fmt.Errorf("k8s.overlay: %w", err)
		}
		docs = append(docs, extraDocs...)
	}

	for i, patchVal := range patchesList {
		docs, err = applyOverlayPatch(thread, docs, patchVal, i)
		if err != nil {
			return nil, fmt.Errorf("k8s.overlay: %w", err)
		}
	}

	if namespace != "" {
		for _, doc := range docs {
			kind, _ := doc["kind"].(string)
			if clusterScopedKinds[kind] {
				continue
			}
			meta := ensureMapField(doc, "metadata")
			meta["namespace"] = namespace
		}
	}

	if len(labelsMap) > 0 {
		for _, doc := range docs {
			meta := ensureMapField(doc, "metadata")
			existing, _ := meta["labels"].(map[string]any)
			meta["labels"] = yamllib.MergeMaps(existing, labelsMap, "replace", "")
		}
	}

	if len(annotationsMap) > 0 {
		for _, doc := range docs {
			meta := ensureMapField(doc, "metadata")
			existing, _ := meta["annotations"].(map[string]any)
			meta["annotations"] = yamllib.MergeMaps(existing, annotationsMap, "replace", "")
		}
	}

	if len(imageSpecs) > 0 {
		for _, doc := range docs {
			rewriteDocImages(doc, imageSpecs)
		}
	}

	if namePrefix != "" || nameSuffix != "" {
		renames := make(map[string]map[string]string)
		for _, doc := range docs {
			kind, _ := doc["kind"].(string)
			if kind == "Namespace" {
				continue
			}
			meta, ok := doc["metadata"].(map[string]any)
			if !ok {
				continue
			}
			oldName, ok := meta["name"].(string)
			if !ok || oldName == "" {
				continue
			}
			newName := namePrefix + oldName + nameSuffix
			meta["name"] = newName
			if renames[kind] == nil {
				renames[kind] = make(map[string]string)
			}
			renames[kind][oldName] = newName
		}
		for _, doc := range docs {
			rewriteDocReferences(doc, renames)
		}
	}

	elems := make([]starlark.Value, len(docs))
	for i, doc := range docs {
		sv, err := goToStarlarkDeep(doc)
		if err != nil {
			return nil, fmt.Errorf("k8s.overlay: output[%d]: %w", i, err)
		}
		elems[i] = sv
	}
	return starlark.NewList(elems), nil
}

func starlarkSequence(v starlark.Value, name string) ([]starlark.Value, error) {
	switch seq := v.(type) {
	case *starlark.List:
		out := make([]starlark.Value, seq.Len())
		for i := 0; i < seq.Len(); i++ {
			out[i] = seq.Index(i)
		}
		return out, nil
	case starlark.Tuple:
		out := make([]starlark.Value, len(seq))
		copy(out, seq)
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be a list or tuple, got %s", name, v.Type())
	}
}

func extractNormalizedMap(v starlark.Value) (map[string]any, error) {
	m, err := extractMap(v)
	if err != nil {
		return nil, err
	}
	return yamllib.DeepCopy(m).(map[string]any), nil
}

func normalizeOverlayInputs(v starlark.Value, name string) ([]map[string]any, error) {
	if s, ok := v.(starlark.String); ok {
		return decodeMultiDocYAML(string(s), name)
	}
	items, err := starlarkSequence(v, name)
	if err != nil {
		return nil, err
	}
	var docs []map[string]any
	for i, item := range items {
		if item == starlark.None {
			continue
		}
		if s, ok := item.(starlark.String); ok {
			multi, err := decodeMultiDocYAML(string(s), fmt.Sprintf("%s[%d]", name, i))
			if err != nil {
				return nil, err
			}
			docs = append(docs, multi...)
			continue
		}
		m, err := extractNormalizedMap(item)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", name, i, err)
		}
		docs = append(docs, m)
	}
	return docs, nil
}

func decodeMultiDocYAML(raw, label string) ([]map[string]any, error) {
	dec := goyaml.NewDecoder(bytes.NewReader([]byte(raw)))
	var docs []map[string]any
	for {
		var val any
		if err := dec.Decode(&val); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("%s: invalid YAML: %w", label, err)
		}
		if val == nil {
			continue
		}
		copied := yamllib.DeepCopy(val)
		m, ok := copied.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: YAML document must be a mapping, got %T", label, val)
		}
		docs = append(docs, m)
	}
	return docs, nil
}

func applyOverlayPatch(thread *starlark.Thread, docs []map[string]any, patchVal starlark.Value, idx int) ([]map[string]any, error) {
	if fn, ok := patchVal.(starlark.Callable); ok {
		out := make([]map[string]any, 0, len(docs))
		for i, doc := range docs {
			arg, err := goToStarlarkDeep(doc)
			if err != nil {
				return nil, fmt.Errorf("patches[%d] on doc[%d]: %w", idx, i, err)
			}
			res, err := starlark.Call(thread, fn, starlark.Tuple{arg}, nil)
			if err != nil {
				return nil, fmt.Errorf("patches[%d] (%s): %w", idx, fn.Name(), err)
			}
			if res == starlark.None {
				continue
			}
			updated, err := extractNormalizedMap(res)
			if err != nil {
				return nil, fmt.Errorf("patches[%d] (%s) return value: %w", idx, fn.Name(), err)
			}
			out = append(out, updated)
		}
		return out, nil
	}

	spec, err := extractNormalizedMap(patchVal)
	if err != nil {
		return nil, fmt.Errorf("patches[%d]: expected function or dict, got %s", idx, patchVal.Type())
	}
	patchMap, ok := spec["patch"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("patches[%d]: dict patch must contain a 'patch' mapping", idx)
	}
	matchKind, _ := spec["kind"].(string)
	matchName, _ := spec["name"].(string)

	out := make([]map[string]any, len(docs))
	for i, doc := range docs {
		if matchesPatchTarget(doc, matchKind, matchName) {
			out[i] = yamllib.MergeMaps(doc, patchMap, "by_key", "name")
		} else {
			out[i] = doc
		}
	}
	return out, nil
}

func matchesPatchTarget(doc map[string]any, kind, name string) bool {
	if kind != "" {
		docKind, _ := doc["kind"].(string)
		if !strings.EqualFold(docKind, kind) {
			return false
		}
	}
	if name != "" {
		meta, _ := doc["metadata"].(map[string]any)
		docName, _ := meta["name"].(string)
		if docName != name {
			return false
		}
	}
	return true
}

func parseImageSpecs(v starlark.Value) (map[string]overlayImageSpec, error) {
	raw, err := extractNormalizedMap(v)
	if err != nil {
		return nil, err
	}
	specs := make(map[string]overlayImageSpec, len(raw))
	for key, item := range raw {
		switch tv := item.(type) {
		case string:
			specs[key] = overlayImageSpec{Direct: tv}
		case map[string]any:
			spec := overlayImageSpec{}
			if nn, ok := tv["new_name"].(string); ok {
				spec.NewName = nn
			}
			if nt, ok := tv["new_tag"].(string); ok {
				spec.NewTag = nt
			}
			if dg, ok := tv["digest"].(string); ok {
				spec.Digest = dg
			}
			specs[key] = spec
		default:
			return nil, fmt.Errorf("image rule for %q must be string or dict, got %T", key, item)
		}
	}
	return specs, nil
}

func splitImageRef(img string) (repo, suffix string) {
	if at := strings.Index(img, "@"); at >= 0 {
		return img[:at], img[at:]
	}
	lastSlash := strings.LastIndex(img, "/")
	lastColon := strings.LastIndex(img, ":")
	if lastColon > lastSlash {
		return img[:lastColon], img[lastColon:]
	}
	return img, ""
}

func applyImageSpec(current string, specs map[string]overlayImageSpec) string {
	repo, suffix := splitImageRef(current)
	spec, ok := specs[current]
	if !ok {
		spec, ok = specs[repo]
		if !ok {
			return current
		}
	}
	if spec.Direct != "" {
		return spec.Direct
	}
	targetRepo := repo
	if spec.NewName != "" {
		targetRepo = spec.NewName
	}
	if spec.Digest != "" {
		d := spec.Digest
		if !strings.HasPrefix(d, "@") {
			d = "@" + d
		}
		return targetRepo + d
	}
	if spec.NewTag != "" {
		return targetRepo + ":" + spec.NewTag
	}
	return targetRepo + suffix
}

func extractPodSpec(doc map[string]any) map[string]any {
	kind, _ := doc["kind"].(string)
	spec, _ := doc["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	switch kind {
	case "Pod":
		return spec
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "ReplicaSet":
		tmpl, _ := spec["template"].(map[string]any)
		if tmpl == nil {
			return nil
		}
		podSpec, _ := tmpl["spec"].(map[string]any)
		return podSpec
	case "CronJob":
		jt, _ := spec["jobTemplate"].(map[string]any)
		if jt == nil {
			return nil
		}
		jtSpec, _ := jt["spec"].(map[string]any)
		if jtSpec == nil {
			return nil
		}
		tmpl, _ := jtSpec["template"].(map[string]any)
		if tmpl == nil {
			return nil
		}
		podSpec, _ := tmpl["spec"].(map[string]any)
		return podSpec
	default:
		return nil
	}
}

func rewriteDocImages(doc map[string]any, specs map[string]overlayImageSpec) {
	podSpec := extractPodSpec(doc)
	if podSpec == nil {
		return
	}
	for _, field := range []string{"containers", "initContainers", "ephemeralContainers"} {
		list, ok := podSpec[field].([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			c, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if img, ok := c["image"].(string); ok && img != "" {
				c["image"] = applyImageSpec(img, specs)
			}
		}
	}
}

func lookupRename(renames map[string]map[string]string, kind, oldName string) (string, bool) {
	if oldName == "" {
		return "", false
	}
	if byName, ok := renames[kind]; ok {
		if newName, exists := byName[oldName]; exists {
			return newName, true
		}
	}
	return "", false
}

func rewriteDocReferences(doc map[string]any, renames map[string]map[string]string) {
	if podSpec := extractPodSpec(doc); podSpec != nil {
		if sa, ok := podSpec["serviceAccountName"].(string); ok {
			if renamed, found := lookupRename(renames, "ServiceAccount", sa); found {
				podSpec["serviceAccountName"] = renamed
			}
		}
		for _, field := range []string{"containers", "initContainers", "ephemeralContainers"} {
			containers, _ := podSpec[field].([]any)
			for _, item := range containers {
				c, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if envList, ok := c["env"].([]any); ok {
					for _, eItem := range envList {
						eMap, _ := eItem.(map[string]any)
						vf, _ := eMap["valueFrom"].(map[string]any)
						if vf == nil {
							continue
						}
						if cmRef, ok := vf["configMapKeyRef"].(map[string]any); ok {
							if name, ok := cmRef["name"].(string); ok {
								if renamed, found := lookupRename(renames, "ConfigMap", name); found {
									cmRef["name"] = renamed
								}
							}
						}
						if secRef, ok := vf["secretKeyRef"].(map[string]any); ok {
							if name, ok := secRef["name"].(string); ok {
								if renamed, found := lookupRename(renames, "Secret", name); found {
									secRef["name"] = renamed
								}
							}
						}
					}
				}
				if envFromList, ok := c["envFrom"].([]any); ok {
					for _, efItem := range envFromList {
						efMap, _ := efItem.(map[string]any)
						if efMap == nil {
							continue
						}
						if cmRef, ok := efMap["configMapRef"].(map[string]any); ok {
							if name, ok := cmRef["name"].(string); ok {
								if renamed, found := lookupRename(renames, "ConfigMap", name); found {
									cmRef["name"] = renamed
								}
							}
						}
						if secRef, ok := efMap["secretRef"].(map[string]any); ok {
							if name, ok := secRef["name"].(string); ok {
								if renamed, found := lookupRename(renames, "Secret", name); found {
									secRef["name"] = renamed
								}
							}
						}
					}
				}
			}
		}
		if volumes, ok := podSpec["volumes"].([]any); ok {
			for _, vItem := range volumes {
				vMap, ok := vItem.(map[string]any)
				if !ok {
					continue
				}
				if cmVol, ok := vMap["configMap"].(map[string]any); ok {
					if name, ok := cmVol["name"].(string); ok {
						if renamed, found := lookupRename(renames, "ConfigMap", name); found {
							cmVol["name"] = renamed
						}
					}
				}
				if secVol, ok := vMap["secret"].(map[string]any); ok {
					if name, ok := secVol["secretName"].(string); ok {
						if renamed, found := lookupRename(renames, "Secret", name); found {
							secVol["secretName"] = renamed
						}
					}
				}
				if pvcVol, ok := vMap["persistentVolumeClaim"].(map[string]any); ok {
					if name, ok := pvcVol["claimName"].(string); ok {
						if renamed, found := lookupRename(renames, "PersistentVolumeClaim", name); found {
							pvcVol["claimName"] = renamed
						}
					}
				}
			}
		}
	}

	kind, _ := doc["kind"].(string)
	switch kind {
	case "HorizontalPodAutoscaler":
		spec, _ := doc["spec"].(map[string]any)
		ref, _ := spec["scaleTargetRef"].(map[string]any)
		if ref != nil {
			refKind, _ := ref["kind"].(string)
			refName, _ := ref["name"].(string)
			if renamed, found := lookupRename(renames, refKind, refName); found {
				ref["name"] = renamed
			}
		}
	case "RoleBinding", "ClusterRoleBinding":
		if roleRef, ok := doc["roleRef"].(map[string]any); ok {
			refKind, _ := roleRef["kind"].(string)
			refName, _ := roleRef["name"].(string)
			if renamed, found := lookupRename(renames, refKind, refName); found {
				roleRef["name"] = renamed
			}
		}
		if subjects, ok := doc["subjects"].([]any); ok {
			for _, sItem := range subjects {
				sMap, _ := sItem.(map[string]any)
				if sMap == nil {
					continue
				}
				sKind, _ := sMap["kind"].(string)
				sName, _ := sMap["name"].(string)
				if renamed, found := lookupRename(renames, sKind, sName); found {
					sMap["name"] = renamed
				}
			}
		}
	}
}

func ensureMapField(m map[string]any, key string) map[string]any {
	if existing, ok := m[key].(map[string]any); ok {
		return existing
	}
	fresh := make(map[string]any)
	m[key] = fresh
	return fresh
}

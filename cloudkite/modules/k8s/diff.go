package k8s

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// diff computes a server-side apply dry-run diff against live cluster state.
// Signature: k8s.diff(manifest, namespace="", field_manager="starkite", timeout="")
func (c *K8sClient) diff(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	var manifest starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "manifest", &manifest)
	remaining := args
	if manifest == nil && len(args) > 0 {
		manifest = args[0]
		remaining = args[1:]
	}

	var p struct {
		Namespace    string `name:"namespace"`
		FieldManager string `name:"field_manager"`
		Timeout      string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	if manifest == nil {
		return nil, fmt.Errorf("k8s.diff: missing required argument: manifest")
	}

	fieldManager := p.FieldManager
	if fieldManager == "" {
		fieldManager = "starkite"
	}

	objs, err := parseManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("k8s.diff: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}
	if ns == "" {
		ns = "default"
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.diff: %w", err)
	}
	defer cancel()

	results := make([]starlark.Value, 0, len(objs))
	for _, obj := range objs {
		gvk := obj.GroupVersionKind()
		gvr, namespaced, err := c.resolver.Resolve(gvk.Kind)
		if err != nil {
			return nil, fmt.Errorf("k8s.diff: %w", err)
		}

		objNs := ns
		if n := obj.GetNamespace(); n != "" {
			objNs = n
		}

		// 1. Fetch live object if exists
		var live *unstructuredObj
		if namespaced {
			live, err = c.dynClient.Resource(gvr).Namespace(objNs).Get(ctx, obj.GetName(), metav1.GetOptions{})
		} else {
			live, err = c.dynClient.Resource(gvr).Get(ctx, obj.GetName(), metav1.GetOptions{})
		}
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("k8s.diff: get live %s/%s: %w", objNs, obj.GetName(), err)
		}

		// 2. Perform Server-Side Apply with DryRunAll
		data, err := json.Marshal(obj.Object)
		if err != nil {
			return nil, fmt.Errorf("k8s.diff: marshal: %w", err)
		}

		opts := metav1.PatchOptions{
			FieldManager: fieldManager,
			DryRun:       []string{metav1.DryRunAll},
		}

		var applied *unstructuredObj
		var conflicts []any
		hasDrift := false
		var driftedFields []any
		diffText := ""

		if namespaced {
			applied, err = c.dynClient.Resource(gvr).Namespace(objNs).Patch(ctx, obj.GetName(), types.ApplyPatchType, data, opts)
		} else {
			applied, err = c.dynClient.Resource(gvr).Patch(ctx, obj.GetName(), types.ApplyPatchType, data, opts)
		}

		if err != nil {
			if apierrors.IsConflict(err) {
				hasDrift = true
				conflicts = append(conflicts, err.Error())
			} else {
				return nil, fmt.Errorf("k8s.diff: dry-run apply %s/%s: %w", objNs, obj.GetName(), err)
			}
		}

		if live == nil {
			hasDrift = true
			driftedFields = append(driftedFields, "(new resource)")
			if applied != nil {
				appliedYAML, _ := yaml.Marshal(applied.Object)
				diffText = formatUnifiedDiff("", string(appliedYAML))
			}
		} else if applied != nil {
			driftedFields = findDriftedFields("", live.Object, applied.Object)
			liveClean := cleanObjectForDiff(live.Object)
			appliedClean := cleanObjectForDiff(applied.Object)
			liveYAML, _ := yaml.Marshal(liveClean)
			appliedYAML, _ := yaml.Marshal(appliedClean)

			if string(liveYAML) != string(appliedYAML) {
				hasDrift = true
				diffText = formatUnifiedDiff(string(liveYAML), string(appliedYAML))
			}
		}

		report := NewAttrDict(map[string]any{
			"name":           obj.GetName(),
			"kind":           gvk.Kind,
			"namespace":      objNs,
			"has_drift":      hasDrift,
			"drifted_fields": driftedFields,
			"conflicts":      conflicts,
			"diff":           diffText,
		})
		if live != nil {
			report.data["live"] = unstructuredToAttrDict(live)
		} else {
			report.data["live"] = starlark.None
		}
		if applied != nil {
			report.data["merged"] = unstructuredToAttrDict(applied)
		} else {
			report.data["merged"] = starlark.None
		}

		results = append(results, report)
	}

	if len(results) == 1 {
		return results[0], nil
	}
	return starlark.NewList(results), nil
}

// cleanObjectForDiff strips noisy metadata fields (like managedFields) from comparison.
func cleanObjectForDiff(obj map[string]any) map[string]any {
	clone := make(map[string]any, len(obj))
	maps.Copy(clone, obj)
	if md, ok := clone["metadata"].(map[string]any); ok {
		mdClone := make(map[string]any, len(md))
		for k, v := range md {
			if k != "managedFields" && k != "resourceVersion" && k != "generation" {
				mdClone[k] = v
			}
		}
		clone["metadata"] = mdClone
	}
	return clone
}

// findDriftedFields compares two maps and returns field paths that changed.
func findDriftedFields(prefix string, a, b any) []any {
	var drifted []any
	aMap, aIsMap := a.(map[string]any)
	bMap, bIsMap := b.(map[string]any)

	if aIsMap && bIsMap {
		allKeys := make(map[string]bool)
		for k := range aMap {
			if k != "managedFields" && k != "resourceVersion" && k != "generation" {
				allKeys[k] = true
			}
		}
		for k := range bMap {
			if k != "managedFields" && k != "resourceVersion" && k != "generation" {
				allKeys[k] = true
			}
		}
		for k := range allKeys {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			valA, okA := aMap[k]
			valB, okB := bMap[k]
			if !okA || !okB {
				drifted = append(drifted, path)
			} else if !reflect.DeepEqual(valA, valB) {
				sub := findDriftedFields(path, valA, valB)
				if len(sub) > 0 {
					drifted = append(drifted, sub...)
				} else {
					drifted = append(drifted, path)
				}
			}
		}
		return drifted
	}

	if !reflect.DeepEqual(a, b) && prefix != "" {
		drifted = append(drifted, prefix)
	}
	return drifted
}

// formatUnifiedDiff generates a line-by-line diff between two strings.
func formatUnifiedDiff(oldStr, newStr string) string {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	var sb strings.Builder
	sb.WriteString("--- live\n+++ applied\n")

	// Simple Myers-style line diff
	i, j := 0, 0
	for i < len(oldLines) || j < len(newLines) {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			sb.WriteString("  " + oldLines[i] + "\n")
			i++
			j++
		} else if j < len(newLines) && (i >= len(oldLines) || !containsFrom(oldLines, i, newLines[j])) {
			sb.WriteString("+ " + newLines[j] + "\n")
			j++
		} else if i < len(oldLines) {
			sb.WriteString("- " + oldLines[i] + "\n")
			i++
		}
	}
	return sb.String()
}

func containsFrom(slice []string, start int, val string) bool {
	for i := start; i < len(slice); i++ {
		if slice[i] == val {
			return true
		}
	}
	return false
}

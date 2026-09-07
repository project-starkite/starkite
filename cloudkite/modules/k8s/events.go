package k8s

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type eventInfo struct {
	name            string
	namespace       string
	eventType       string
	reason          string
	message         string
	count           int64
	firstTime       time.Time
	firstTimeStr    string
	lastTime        time.Time
	lastTimeStr     string
	source          string
	regardingKind   string
	regardingName   string
	regardingNS     string
	regardingUID    string
	regardingAPIVer string
	raw             *unstructured.Unstructured
}

// formatAge formats a duration into a human-readable relative age string (e.g., 45s, 2m, 3h, 5d).
func formatAge(d time.Duration) string {
	if d < 0 {
		return "0s"
	}
	seconds := int(d.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := int(d.Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

// parseEventTimestamp parses RFC3339 or RFC3339Nano string timestamps.
func parseEventTimestamp(s string) (time.Time, string) {
	if s == "" {
		return time.Time{}, ""
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), t.UTC().Format(time.RFC3339)
	}
	return time.Time{}, s
}

func extractTimestamp(obj map[string]any, fields ...string) (time.Time, string) {
	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil || val == nil {
		return time.Time{}, ""
	}
	switch v := val.(type) {
	case string:
		return parseEventTimestamp(v)
	case time.Time:
		return v.UTC(), v.UTC().Format(time.RFC3339)
	default:
		return parseEventTimestamp(fmt.Sprint(v))
	}
}

func parseEventInfo(u *unstructured.Unstructured) eventInfo {
	ev := eventInfo{
		name:      u.GetName(),
		namespace: u.GetNamespace(),
		raw:       u,
	}

	ev.eventType, _, _ = unstructured.NestedString(u.Object, "type")
	if ev.eventType == "" {
		ev.eventType = "Normal"
	}

	ev.reason, _, _ = unstructured.NestedString(u.Object, "reason")

	// Message: "note" in events.k8s.io/v1, "message" in core/v1
	ev.message, _, _ = unstructured.NestedString(u.Object, "note")
	if ev.message == "" {
		ev.message, _, _ = unstructured.NestedString(u.Object, "message")
	}

	// Count: series.count or count, default 1
	ev.count, _, _ = unstructured.NestedInt64(u.Object, "series", "count")
	if ev.count == 0 {
		ev.count, _, _ = unstructured.NestedInt64(u.Object, "count")
	}
	if ev.count == 0 {
		ev.count = 1
	}

	// Source: reportingController/reportingInstance or source.component/source.host or reportingComponent
	repCtrl, _, _ := unstructured.NestedString(u.Object, "reportingController")
	repInst, _, _ := unstructured.NestedString(u.Object, "reportingInstance")
	if repCtrl != "" {
		if repInst != "" {
			ev.source = repCtrl + "/" + repInst
		} else {
			ev.source = repCtrl
		}
	} else {
		srcComp, _, _ := unstructured.NestedString(u.Object, "source", "component")
		srcHost, _, _ := unstructured.NestedString(u.Object, "source", "host")
		if srcComp != "" {
			if srcHost != "" {
				ev.source = srcComp + "/" + srcHost
			} else {
				ev.source = srcComp
			}
		} else {
			ev.source, _, _ = unstructured.NestedString(u.Object, "reportingComponent")
		}
	}

	// Target/regarding: "regarding" in events.k8s.io/v1, "involvedObject" in core/v1
	var regardingMap map[string]any
	if m, found, _ := unstructured.NestedMap(u.Object, "regarding"); found && m != nil {
		regardingMap = m
	} else if m, found, _ := unstructured.NestedMap(u.Object, "involvedObject"); found && m != nil {
		regardingMap = m
	}

	if regardingMap != nil {
		if v, ok := regardingMap["kind"].(string); ok {
			ev.regardingKind = v
		}
		if v, ok := regardingMap["name"].(string); ok {
			ev.regardingName = v
		}
		if v, ok := regardingMap["namespace"].(string); ok {
			ev.regardingNS = v
		}
		if v, ok := regardingMap["uid"].(string); ok {
			ev.regardingUID = v
		}
		if v, ok := regardingMap["apiVersion"].(string); ok {
			ev.regardingAPIVer = v
		}
	}

	// Timestamps
	lastTime, lastTimeStr := extractTimestamp(u.Object, "series", "lastObservedTime")
	if lastTime.IsZero() {
		lastTime, lastTimeStr = extractTimestamp(u.Object, "lastTimestamp")
	}
	if lastTime.IsZero() {
		lastTime, lastTimeStr = extractTimestamp(u.Object, "eventTime")
	}
	if lastTime.IsZero() {
		lastTime, lastTimeStr = extractTimestamp(u.Object, "metadata", "creationTimestamp")
	}

	firstTime, firstTimeStr := extractTimestamp(u.Object, "firstTimestamp")
	if firstTime.IsZero() {
		firstTime, firstTimeStr = extractTimestamp(u.Object, "eventTime")
	}
	if firstTime.IsZero() {
		firstTime, firstTimeStr = extractTimestamp(u.Object, "metadata", "creationTimestamp")
	}
	if firstTime.IsZero() && !lastTime.IsZero() {
		firstTime = lastTime
		firstTimeStr = lastTimeStr
	}
	if lastTime.IsZero() && !firstTime.IsZero() {
		lastTime = firstTime
		lastTimeStr = firstTimeStr
	}

	ev.lastTime = lastTime
	ev.lastTimeStr = lastTimeStr
	ev.firstTime = firstTime
	ev.firstTimeStr = firstTimeStr

	return ev
}

// events queries and filters Kubernetes events.
// Signature: k8s.events(target=None, kind="", name="", namespace="", type="", reason="", since="", limit=0, labels="", fields="", timeout="")
func (c *K8sClient) events(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	var targetVal starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "target", &targetVal)
	if targetVal == nil {
		filteredKwargs = filterKwargValue(filteredKwargs, "obj", &targetVal)
	}

	var p struct {
		Target    string `name:"target"`
		Kind      string `name:"kind"`
		Name      string `name:"name"`
		Namespace string `name:"namespace"`
		Type      string `name:"type"`
		Reason    string `name:"reason"`
		Since     string `name:"since"`
		Limit     int    `name:"limit"`
		Labels    string `name:"labels"`
		Fields    string `name:"fields"`
		Timeout   string `name:"timeout"`
	}

	remainingArgs := args
	if targetVal == nil && len(remainingArgs) > 0 {
		switch v := remainingArgs[0].(type) {
		case KubeObject, *AttrDict, *starlark.Dict:
			targetVal = v
			remainingArgs = remainingArgs[1:]
		case starlark.String:
			if len(remainingArgs) >= 2 {
				if s2, ok := remainingArgs[1].(starlark.String); ok {
					p.Kind = string(v)
					p.Name = string(s2)
					remainingArgs = remainingArgs[2:]
				} else {
					targetVal = v
					remainingArgs = remainingArgs[1:]
				}
			} else {
				targetVal = v
				remainingArgs = remainingArgs[1:]
			}
		}
	}

	if err := startype.Args(remainingArgs, filteredKwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.events: %w", err)
	}

	if p.Target != "" && p.Name == "" {
		p.Name = p.Target
	}

	var targetUID string
	if targetVal != nil {
		if s, ok := targetVal.(starlark.String); ok {
			if p.Name == "" {
				p.Name = string(s)
			}
		} else {
			u, err := toUnstructuredObj(targetVal)
			if err != nil {
				return nil, fmt.Errorf("k8s.events: target: %w", err)
			}
			if p.Kind == "" {
				p.Kind = u.GetKind()
			}
			if p.Name == "" {
				p.Name = u.GetName()
			}
			if p.Namespace == "" {
				p.Namespace = u.GetNamespace()
			}
			targetUID = string(u.GetUID())
		}
	}

	var sinceDuration time.Duration
	if p.Since != "" {
		dur, err := time.ParseDuration(p.Since)
		if err != nil {
			return nil, fmt.Errorf("k8s.events: invalid 'since' duration %q: %w", p.Since, err)
		}
		if dur <= 0 {
			return nil, fmt.Errorf("k8s.events: 'since' duration must be positive, got %s", p.Since)
		}
		sinceDuration = dur
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.events: %w", err)
	}
	defer cancel()

	eventsV1GVR := schema.GroupVersionResource{Group: "events.k8s.io", Version: "v1", Resource: "events"}
	coreV1GVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

	listOpts := metav1.ListOptions{}
	if p.Labels != "" {
		listOpts.LabelSelector = p.Labels
	}
	if p.Fields != "" {
		listOpts.FieldSelector = p.Fields
	}

	var rawItems []unstructured.Unstructured
	var queryErr error

	// 1. Query events.k8s.io/v1 first
	var list *unstructured.UnstructuredList
	if ns != "" && ns != "all" {
		list, queryErr = c.dynClient.Resource(eventsV1GVR).Namespace(ns).List(ctx, listOpts)
	} else {
		list, queryErr = c.dynClient.Resource(eventsV1GVR).List(ctx, listOpts)
	}

	if queryErr == nil && list != nil && len(list.Items) > 0 {
		rawItems = list.Items
	} else {
		// 2. Fall back to core/v1 if events.k8s.io/v1 returned error or 0 items
		var coreList *unstructured.UnstructuredList
		var coreErr error
		if ns != "" && ns != "all" {
			coreList, coreErr = c.dynClient.Resource(coreV1GVR).Namespace(ns).List(ctx, listOpts)
		} else {
			coreList, coreErr = c.dynClient.Resource(coreV1GVR).List(ctx, listOpts)
		}
		if coreErr == nil && coreList != nil && len(coreList.Items) > 0 {
			rawItems = coreList.Items
		} else if queryErr != nil && coreErr != nil {
			return nil, fmt.Errorf("k8s.events: query failed: %w", queryErr)
		} else if list != nil {
			rawItems = list.Items
		} else if coreList != nil {
			rawItems = coreList.Items
		}
	}

	now := time.Now()
	var cutoff time.Time
	if sinceDuration > 0 {
		cutoff = now.Add(-sinceDuration)
	}

	var filtered []eventInfo
	for i := range rawItems {
		ev := parseEventInfo(&rawItems[i])

		// UID filter
		if targetUID != "" && ev.regardingUID != "" {
			if targetUID != ev.regardingUID {
				continue
			}
		}

		// Kind filter
		if p.Kind != "" {
			if !strings.EqualFold(ev.regardingKind, p.Kind) {
				continue
			}
		}

		// Name filter
		if p.Name != "" {
			if ev.regardingName != p.Name {
				continue
			}
		}

		// Type filter
		if p.Type != "" {
			if !strings.EqualFold(ev.eventType, p.Type) {
				continue
			}
		}

		// Reason filter (substring match)
		if p.Reason != "" {
			if !strings.Contains(strings.ToLower(ev.reason), strings.ToLower(p.Reason)) {
				continue
			}
		}

		// Time window filter
		if sinceDuration > 0 {
			if ev.lastTime.IsZero() || ev.lastTime.Before(cutoff) {
				continue
			}
		}

		filtered = append(filtered, ev)
	}

	// Sort newest first by lastTime descending, then firstTime descending, then name ascending
	sort.Slice(filtered, func(i, j int) bool {
		if !filtered[i].lastTime.Equal(filtered[j].lastTime) {
			return filtered[i].lastTime.After(filtered[j].lastTime)
		}
		if !filtered[i].firstTime.Equal(filtered[j].firstTime) {
			return filtered[i].firstTime.After(filtered[j].firstTime)
		}
		return filtered[i].name < filtered[j].name
	})

	// Limit
	if p.Limit > 0 && len(filtered) > p.Limit {
		filtered = filtered[:p.Limit]
	}

	result := make([]starlark.Value, 0, len(filtered))
	for _, ev := range filtered {
		age := ""
		if !ev.lastTime.IsZero() {
			age = formatAge(now.Sub(ev.lastTime))
		}

		regardingDict := NewAttrDict(map[string]any{
			"kind":        ev.regardingKind,
			"name":        ev.regardingName,
			"namespace":   ev.regardingNS,
			"uid":         ev.regardingUID,
			"apiVersion":  ev.regardingAPIVer,
			"api_version": ev.regardingAPIVer,
		})

		rawDict := unstructuredToAttrDict(ev.raw)

		evDict := NewAttrDict(map[string]any{
			"name":            ev.name,
			"namespace":       ev.namespace,
			"type":            ev.eventType,
			"reason":          ev.reason,
			"message":         ev.message,
			"count":           ev.count,
			"first_time":      ev.firstTimeStr,
			"first_timestamp": ev.firstTimeStr,
			"last_time":       ev.lastTimeStr,
			"last_timestamp":  ev.lastTimeStr,
			"age":             age,
			"source":          ev.source,
			"regarding":       regardingDict,
			"involved_object": regardingDict,
			"raw":             rawDict,
		})
		result = append(result, evDict)
	}

	return starlark.NewList(result), nil
}

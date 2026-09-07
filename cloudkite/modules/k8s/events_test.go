package k8s

import (
	"testing"
	"time"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{-5 * time.Second, "0s"},
		{0, "0s"},
		{45 * time.Second, "45s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{2 * time.Minute, "2m"},
		{59 * time.Minute, "59m"},
		{60 * time.Minute, "1h"},
		{3 * time.Hour, "3h"},
		{23 * time.Hour, "23h"},
		{24 * time.Hour, "1d"},
		{72 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		got := formatAge(tt.d)
		if got != tt.expected {
			t.Errorf("formatAge(%v) = %q, want %q", tt.d, got, tt.expected)
		}
	}
}

func TestParseEventTimestamp(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rfc3339Str := now.Format(time.RFC3339)
	rfc3339NanoStr := now.Format(time.RFC3339Nano)

	parsed1, s1 := parseEventTimestamp(rfc3339Str)
	if !parsed1.Equal(now) || s1 != rfc3339Str {
		t.Errorf("parseEventTimestamp(%s) = %v, %s; want %v, %s", rfc3339Str, parsed1, s1, now, rfc3339Str)
	}

	parsed2, s2 := parseEventTimestamp(rfc3339NanoStr)
	if !parsed2.Equal(now) || s2 != rfc3339Str {
		t.Errorf("parseEventTimestamp(%s) = %v, %s; want %v, %s", rfc3339NanoStr, parsed2, s2, now, rfc3339Str)
	}

	parsedZero, sZero := parseEventTimestamp("")
	if !parsedZero.IsZero() || sZero != "" {
		t.Errorf("parseEventTimestamp(\"\") = %v, %q; want zero time and empty string", parsedZero, sZero)
	}
}

func TestEventsEventsV1QueryAndFilter(t *testing.T) {
	eventsV1GVR := schema.GroupVersionResource{Group: "events.k8s.io", Version: "v1", Resource: "events"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		eventsV1GVR: "EventList",
	})
	ctx := t.Context()

	now := time.Now().UTC()
	t1 := now.Add(-10 * time.Minute).Format(time.RFC3339)
	t2 := now.Add(-5 * time.Minute).Format(time.RFC3339)
	t3 := now.Add(-2 * time.Hour).Format(time.RFC3339)

	// Create events in events.k8s.io/v1
	ev1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "events.k8s.io/v1",
			"kind":       "Event",
			"metadata": map[string]any{
				"name":      "ev1",
				"namespace": "production",
			},
			"type":   "Warning",
			"reason": "BackOff",
			"note":   "Back-off restarting failed container",
			"series": map[string]any{
				"count":            int64(7),
				"lastObservedTime": t2,
			},
			"eventTime":           t1,
			"reportingController": "k8s.io/kubelet",
			"reportingInstance":   "node-1",
			"regarding": map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"name":       "llm-worker-1",
				"namespace":  "production",
				"uid":        "uid-pod-1",
			},
		},
	}

	ev2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "events.k8s.io/v1",
			"kind":       "Event",
			"metadata": map[string]any{
				"name":      "ev2",
				"namespace": "production",
			},
			"type":                "Normal",
			"reason":              "Scheduled",
			"note":                "Successfully assigned production/llm-worker-1 to node-1",
			"count":               int64(1),
			"lastTimestamp":       t1,
			"reportingController": "default-scheduler",
			"regarding": map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"name":       "llm-worker-1",
				"namespace":  "production",
				"uid":        "uid-pod-1",
			},
		},
	}

	ev3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "events.k8s.io/v1",
			"kind":       "Event",
			"metadata": map[string]any{
				"name":      "ev3",
				"namespace": "production",
			},
			"type":                "Warning",
			"reason":              "FailedMount",
			"note":                "MountVolume.SetUp failed for volume",
			"count":               int64(3),
			"lastTimestamp":       t3, // 2 hours ago
			"reportingController": "k8s.io/kubelet",
			"regarding": map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"name":       "other-pod",
				"namespace":  "production",
				"uid":        "uid-pod-2",
			},
		},
	}

	for _, ev := range []*unstructured.Unstructured{ev1, ev2, ev3} {
		_, err := fakeClient.Resource(eventsV1GVR).Namespace("production").Create(ctx, ev, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to seed event: %v", err)
		}
	}

	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  NewResolver(nil),
		namespace: "production",
	}
	thread := &starlark.Thread{Name: "test-thread"}

	// 1. Filter by target object: pass AttrDict representing llm-worker-1
	targetObj := unstructuredToAttrDict(&unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "llm-worker-1",
				"namespace": "production",
				"uid":       "uid-pod-1",
			},
		},
	})

	resVal, err := kClient.events(thread, nil, starlark.Tuple{targetObj}, nil)
	if err != nil {
		t.Fatalf("events(targetObj) failed: %v", err)
	}
	list, ok := resVal.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", resVal)
	}
	if list.Len() != 2 {
		t.Fatalf("expected 2 events for targetObj, got %d", list.Len())
	}

	// Verify order: ev1 (5m ago) should be before ev2 (10m ago)
	firstEv := list.Index(0).(*AttrDict)
	reasonVal, _ := firstEv.Attr("reason")
	if reasonVal != starlark.String("BackOff") {
		t.Errorf("expected first event reason BackOff, got %v", reasonVal)
	}
	msgVal, _ := firstEv.Attr("message")
	if msgVal != starlark.String("Back-off restarting failed container") {
		t.Errorf("expected first event message, got %v", msgVal)
	}
	countVal, _ := firstEv.Attr("count")
	if countVal != starlark.MakeInt64(7) {
		t.Errorf("expected first event count 7, got %v", countVal)
	}
	srcVal, _ := firstEv.Attr("source")
	if srcVal != starlark.String("k8s.io/kubelet/node-1") {
		t.Errorf("expected first event source k8s.io/kubelet/node-1, got %v", srcVal)
	}
	ageVal, _ := firstEv.Attr("age")
	if ageStr, ok := ageVal.(starlark.String); !ok || string(ageStr) == "" {
		t.Errorf("expected formatted age string, got %v", ageVal)
	}

	// 2. Filter by type = "Warning"
	resWarn, err := kClient.events(thread, nil, nil, []starlark.Tuple{
		{starlark.String("type"), starlark.String("Warning")},
		{starlark.String("namespace"), starlark.String("production")},
	})
	if err != nil {
		t.Fatalf("events(type=Warning) failed: %v", err)
	}
	warnList := resWarn.(*starlark.List)
	if warnList.Len() != 2 { // ev1 and ev3
		t.Fatalf("expected 2 warning events, got %d", warnList.Len())
	}

	// 3. Filter by since = "30m"
	resSince, err := kClient.events(thread, nil, nil, []starlark.Tuple{
		{starlark.String("since"), starlark.String("30m")},
		{starlark.String("namespace"), starlark.String("production")},
	})
	if err != nil {
		t.Fatalf("events(since=30m) failed: %v", err)
	}
	sinceList := resSince.(*starlark.List)
	if sinceList.Len() != 2 { // ev1 (5m) and ev2 (10m), ev3 (2h) excluded
		t.Fatalf("expected 2 recent events within 30m, got %d", sinceList.Len())
	}

	// 4. Filter by reason substring
	resReason, err := kClient.events(thread, nil, nil, []starlark.Tuple{
		{starlark.String("reason"), starlark.String("back")},
		{starlark.String("namespace"), starlark.String("production")},
	})
	if err != nil {
		t.Fatalf("events(reason=back) failed: %v", err)
	}
	reasonList := resReason.(*starlark.List)
	if reasonList.Len() != 1 {
		t.Fatalf("expected 1 event matching reason 'back', got %d", reasonList.Len())
	}

	// 5. Limit = 1
	resLimit, err := kClient.events(thread, nil, nil, []starlark.Tuple{
		{starlark.String("limit"), starlark.MakeInt(1)},
		{starlark.String("namespace"), starlark.String("production")},
	})
	if err != nil {
		t.Fatalf("events(limit=1) failed: %v", err)
	}
	limitList := resLimit.(*starlark.List)
	if limitList.Len() != 1 {
		t.Fatalf("expected 1 event with limit=1, got %d", limitList.Len())
	}
}

func TestEventsCoreV1Fallback(t *testing.T) {
	eventsV1GVR := schema.GroupVersionResource{Group: "events.k8s.io", Version: "v1", Resource: "events"}
	coreV1GVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		eventsV1GVR: "EventList",
		coreV1GVR:   "EventList",
	})
	ctx := t.Context()

	now := time.Now().UTC()
	t1 := now.Add(-3 * time.Minute).Format(time.RFC3339)

	// Seed only in core/v1, leave events.k8s.io/v1 empty
	coreEv := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Event",
			"metadata": map[string]any{
				"name":      "core-ev-1",
				"namespace": "default",
			},
			"type":          "Warning",
			"reason":        "OOMKilled",
			"message":       "Container killed by OOM killer",
			"count":         int64(2),
			"lastTimestamp": t1,
			"source": map[string]any{
				"component": "kubelet",
				"host":      "node-worker-2",
			},
			"involvedObject": map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"name":       "batch-job-xyz",
				"namespace":  "default",
				"uid":        "uid-core-1",
			},
		},
	}

	_, err := fakeClient.Resource(coreV1GVR).Namespace("default").Create(ctx, coreEv, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to seed core/v1 event: %v", err)
	}

	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  NewResolver(nil),
		namespace: "default",
	}
	thread := &starlark.Thread{Name: "test-thread"}

	// Query events — should fall back to core/v1 seamlessly
	resVal, err := kClient.events(thread, nil, nil, []starlark.Tuple{
		{starlark.String("kind"), starlark.String("Pod")},
		{starlark.String("name"), starlark.String("batch-job-xyz")},
	})
	if err != nil {
		t.Fatalf("events fallback failed: %v", err)
	}
	list := resVal.(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 event from core/v1 fallback, got %d", list.Len())
	}

	ev := list.Index(0).(*AttrDict)
	reason, _ := ev.Attr("reason")
	if reason != starlark.String("OOMKilled") {
		t.Errorf("expected reason OOMKilled, got %v", reason)
	}
	msg, _ := ev.Attr("message")
	if msg != starlark.String("Container killed by OOM killer") {
		t.Errorf("expected message from core/v1, got %v", msg)
	}
	src, _ := ev.Attr("source")
	if src != starlark.String("kubelet/node-worker-2") {
		t.Errorf("expected source kubelet/node-worker-2, got %v", src)
	}

	// Check involved_object / regarding
	regVal, _ := ev.Attr("regarding")
	regDict := regVal.(*AttrDict)
	regKind, _ := regDict.Attr("kind")
	if regKind != starlark.String("Pod") {
		t.Errorf("expected regarding.kind Pod, got %v", regKind)
	}
	regName, _ := regDict.Attr("name")
	if regName != starlark.String("batch-job-xyz") {
		t.Errorf("expected regarding.name batch-job-xyz, got %v", regName)
	}
}

func TestEventsInvalidSinceError(t *testing.T) {
	kClient := &K8sClient{
		dynClient: nil,
		resolver:  NewResolver(nil),
		namespace: "default",
	}
	thread := &starlark.Thread{Name: "test-thread"}

	// Invalid duration
	_, err := kClient.events(thread, nil, nil, []starlark.Tuple{
		{starlark.String("since"), starlark.String("not-a-duration")},
	})
	if err == nil {
		t.Fatal("expected error for invalid 'since' duration")
	}

	// Negative duration
	_, err = kClient.events(thread, nil, nil, []starlark.Tuple{
		{starlark.String("since"), starlark.String("-5m")},
	})
	if err == nil {
		t.Fatal("expected error for negative 'since' duration")
	}
}

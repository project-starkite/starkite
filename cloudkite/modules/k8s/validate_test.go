package k8s

import (
	"testing"

	"go.starlark.net/starlark"
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
	ad, ok := val.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict, got %T", val)
	}
	m := ad.ToMap()
	if valid, _ := m["valid"].(bool); !valid {
		t.Fatalf("expected valid=true, got %v", m["valid"])
	}

	// Invalid expression: replicas > 5
	val2, err := runValidate(thread, nil, starlark.Tuple{}, []starlark.Tuple{
		{starlark.String("manifest"), NewAttrDict(manifest)},
		{starlark.String("expression"), starlark.String("object.spec.replicas > 5")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ad2, ok := val2.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict, got %T", val2)
	}
	m2 := ad2.ToMap()
	if valid, _ := m2["valid"].(bool); valid {
		t.Fatalf("expected valid=false, got %v", m2["valid"])
	}
	violations, _ := m2["violations"].([]any)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %v", violations)
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
	ad := val.(*AttrDict)
	m := ad.ToMap()
	if valid, _ := m["valid"].(bool); !valid {
		t.Fatalf("expected valid=true, got %v", m["violations"])
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
	adPriv := valPriv.(*AttrDict)
	mPriv := adPriv.ToMap()
	if valid, _ := mPriv["valid"].(bool); valid {
		t.Fatalf("expected valid=false for privileged container, got %v", mPriv["valid"])
	}
	violations, _ := mPriv["violations"].([]any)
	if len(violations) != 1 || violations[0] != "Privileged containers are not allowed" {
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
	ad := val.(*AttrDict)
	mDev := ad.ToMap()
	if valid, _ := mDev["valid"].(bool); !valid {
		t.Fatalf("expected valid=true because matchCondition skipped policy, got %v", mDev["violations"])
	}
}

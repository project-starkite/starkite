package k8s

import (
	"testing"
)

func TestResolveBuiltinKinds(t *testing.T) {
	resolver := NewResolver(nil) // nil discovery — only builtins

	tests := []struct {
		kind       string
		wantGroup  string
		wantRes    string
		wantNS     bool
	}{
		{"pod", "", "pods", true},
		{"pods", "", "pods", true},
		{"service", "", "services", true},
		{"svc", "", "services", true},
		{"deployment", "apps", "deployments", true},
		{"deploy", "apps", "deployments", true},
		{"statefulset", "apps", "statefulsets", true},
		{"sts", "apps", "statefulsets", true},
		{"daemonset", "apps", "daemonsets", true},
		{"ds", "apps", "daemonsets", true},
		{"configmap", "", "configmaps", true},
		{"cm", "", "configmaps", true},
		{"secret", "", "secrets", true},
		{"namespace", "", "namespaces", false},
		{"ns", "", "namespaces", false},
		{"node", "", "nodes", false},
		{"nodes", "", "nodes", false},
		{"job", "batch", "jobs", true},
		{"cronjob", "batch", "cronjobs", true},
		{"ingress", "networking.k8s.io", "ingresses", true},
		{"hpa", "autoscaling", "horizontalpodautoscalers", true},
		{"role", "rbac.authorization.k8s.io", "roles", true},
		{"clusterrole", "rbac.authorization.k8s.io", "clusterroles", false},
		{"pvc", "", "persistentvolumeclaims", true},
		{"sa", "", "serviceaccounts", true},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			gvr, ns, err := resolver.Resolve(tt.kind)
			if err != nil {
				t.Fatalf("Resolve(%q) error: %v", tt.kind, err)
			}
			if gvr.Group != tt.wantGroup {
				t.Errorf("Group = %q, want %q", gvr.Group, tt.wantGroup)
			}
			if gvr.Resource != tt.wantRes {
				t.Errorf("Resource = %q, want %q", gvr.Resource, tt.wantRes)
			}
			if ns != tt.wantNS {
				t.Errorf("Namespaced = %v, want %v", ns, tt.wantNS)
			}
		})
	}
}

func TestResolveCaseInsensitive(t *testing.T) {
	resolver := NewResolver(nil)

	gvr, _, err := resolver.Resolve("POD")
	if err != nil {
		t.Fatalf("Resolve(POD) error: %v", err)
	}
	if gvr.Resource != "pods" {
		t.Errorf("Resource = %q, want %q", gvr.Resource, "pods")
	}
}

func TestResolveQualified(t *testing.T) {
	resolver := NewResolver(nil)

	gvr, ns, err := resolver.Resolve("apps/deployments")
	if err != nil {
		t.Fatalf("Resolve(apps/deployments) error: %v", err)
	}
	if gvr.Group != "apps" {
		t.Errorf("Group = %q, want %q", gvr.Group, "apps")
	}
	if gvr.Resource != "deployments" {
		t.Errorf("Resource = %q, want %q", gvr.Resource, "deployments")
	}
	if !ns {
		t.Error("Namespaced = false, want true")
	}
}

func TestResolveUnknownKind(t *testing.T) {
	resolver := NewResolver(nil) // nil discovery

	_, _, err := resolver.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error for unknown kind with no discovery")
	}
}

func TestResolveWhitespace(t *testing.T) {
	resolver := NewResolver(nil)

	gvr, _, err := resolver.Resolve("  pod  ")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if gvr.Resource != "pods" {
		t.Errorf("Resource = %q, want %q", gvr.Resource, "pods")
	}
}

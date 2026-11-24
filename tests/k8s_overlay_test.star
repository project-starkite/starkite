# k8s_overlay_test.star - Offline tests for k8s.obj cloning and k8s.overlay()

def test_obj_clone_kube_resource_and_map_merge():
    """Test k8s.obj.<kind>() cloning an existing KubeResource and merging map fields."""
    base_cm = k8s.obj.config_map(
        name = "the-map",
        labels = {"app": "hello", "tier": "web"},
        data = {"altGreeting": "Good Morning!", "enableRisky": "false"},
    )
    staging_cm = k8s.obj.config_map(
        base_cm,
        labels = {"environment": "staging", "tier": None},
        data = {"altGreeting": "Have a pineapple!"},
    )

    base_dict = base_cm.to_dict()
    staging_dict = staging_cm.to_dict()
    assert(base_dict["data"]["altGreeting"] == "Good Morning!", "base_cm should not be mutated")
    assert(staging_dict["data"]["altGreeting"] == "Have a pineapple!", "override data key")
    assert(staging_dict["data"]["enableRisky"] == "false", "preserve untouched base data key")
    assert("app" in staging_dict["metadata"]["labels"], "preserve untouched base label")
    assert("tier" not in staging_dict["metadata"]["labels"], "None deletes base label")
    assert(staging_dict["metadata"]["labels"]["environment"] == "staging", "add override label")

    base_dep = k8s.obj.deployment(
        name = "the-deployment",
        replicas = 1,
        labels = {"app": "hello"},
        containers = [{"name": "app", "image": "monopole/hello:1"}],
    )
    prod_dep = k8s.obj.deployment(base_dep, replicas = 3, labels = {"environment": "production"})
    prod_dict = prod_dep.to_dict()
    assert(base_dep.to_dict()["spec"]["replicas"] == 1, "base_dep should remain replicas=1")
    assert(prod_dict["spec"]["replicas"] == 3, "prod_dep should have replicas=3")
    assert(prod_dict["spec"]["template"]["spec"]["containers"][0]["image"] == "monopole/hello:1", "preserve base pod template")

def test_overlay_namespace_prefix_refs_images_and_patches():
    """Test k8s.overlay() transforms, cross-reference rewriting, extra resources, and KYAML."""
    base_sa = k8s.obj.service_account(name = "hello-sa")
    base_cm = k8s.obj.config_map(
        name = "the-map",
        data = {"altGreeting": "Good Morning!", "enableRisky": "false"},
    )
    base_dep = k8s.obj.deployment(
        name = "the-deployment",
        replicas = 1,
        labels = {"app": "hello"},
        service_account = base_sa,
        containers = [{
            "name": "the-container",
            "image": "monopole/hello:1",
            "env": [
                {"name": "ALT_GREETING", "valueFrom": {"configMapKeyRef": {"name": "the-map", "key": "altGreeting"}}},
            ],
        }],
    )
    base_svc = k8s.obj.service(
        name = "the-service",
        selector = {"app": "hello"},
        ports = [{"port": 8666, "targetPort": 8080}],
    )

    def harden_containers(obj):
        if obj.get("kind") != "Deployment":
            return obj
        for c in obj["spec"]["template"]["spec"]["containers"]:
            c["securityContext"] = {"runAsNonRoot": True, "readOnlyRootFilesystem": True}
        return obj

    out = k8s.overlay(
        [base_sa, base_cm, base_dep, base_svc],
        namespace = "hello-prod",
        name_prefix = "prod-",
        labels = {"environment": "production"},
        annotations = {"managed-by": "starkite"},
        images = {"monopole/hello": "registry.example.internal/monopole/hello:2"},
        patches = [
            {"kind": "Deployment", "name": "the-deployment", "patch": {"spec": {"replicas": 3}}},
            {"kind": "ConfigMap", "name": "the-map", "patch": {"data": {"altGreeting": "Hello Prod!"}}},
            harden_containers,
        ],
        extra = [
            k8s.obj.namespace(name = "hello-prod"),
            k8s.obj.horizontal_pod_autoscaler(
                name = "the-hpa",
                scale_target_ref = {"apiVersion": "apps/v1", "kind": "Deployment", "name": "the-deployment"},
                min_replicas = 3,
                max_replicas = 6,
            ),
        ],
    )

    assert(len(out) == 6, "expected 6 overlay resources")
    by_kind = {d["kind"]: d for d in out}

    ns = by_kind["Namespace"]
    assert(ns["metadata"]["name"] == "hello-prod", "Namespace should not be prefixed")
    assert("namespace" not in ns["metadata"], "Namespace should not receive metadata.namespace")

    dep = by_kind["Deployment"]
    assert(dep["metadata"]["name"] == "prod-the-deployment", "Deployment should be prefixed")
    assert(dep["metadata"]["namespace"] == "hello-prod", "Deployment should be namespaced")
    assert(dep["spec"]["replicas"] == 3, "Deployment replicas patched to 3")
    assert(dep["spec"]["template"]["spec"]["serviceAccountName"] == "prod-hello-sa", "serviceAccountName rewritten")

    c0 = dep["spec"]["template"]["spec"]["containers"][0]
    assert(c0["image"] == "registry.example.internal/monopole/hello:2", "image rewritten")
    assert(c0["securityContext"]["runAsNonRoot"] == True, "securityContext injected")
    assert(c0["env"][0]["valueFrom"]["configMapKeyRef"]["name"] == "prod-the-map", "configMapKeyRef rewritten")

    hpa = by_kind["HorizontalPodAutoscaler"]
    assert(hpa["metadata"]["name"] == "prod-the-hpa", "HPA should be prefixed")
    assert(hpa["spec"]["scaleTargetRef"]["name"] == "prod-the-deployment", "HPA scaleTargetRef rewritten")

    rendered = k8s.yaml(out)
    assert('enableRisky: "false"' in rendered, "KYAML should quote boolean strings")

def test_overlay_drop_resource_and_try_variant():
    """Test dropping resources via patch function returning None, dict image spec, and k8s.try_overlay."""
    raw_yaml = """apiVersion: v1
kind: ConfigMap
metadata:
  name: drop-me
data:
  a: "1"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: web
          image: localhost:5000/team/web:1.0
"""

    def drop_cm(obj):
        if obj.get("kind") == "ConfigMap":
            return None
        return obj

    out = k8s.overlay(
        raw_yaml,
        images = {"localhost:5000/team/web": {"new_tag": "2.0"}},
        patches = [drop_cm],
    )
    assert(len(out) == 1, "ConfigMap should be dropped")
    assert(out[0]["spec"]["template"]["spec"]["containers"][0]["image"] == "localhost:5000/team/web:2.0", "tag updated")

    bad = k8s.try_overlay("not-a-map")
    assert(not bad.ok, "k8s.try_overlay should fail on invalid YAML mapping")

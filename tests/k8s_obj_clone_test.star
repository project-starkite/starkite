# k8s_obj_clone_test.star - Offline tests for k8s.obj.<kind>() cloning and map merging

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

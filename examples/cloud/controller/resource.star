#!/usr/bin/env cloudkite
# resource.star — Define the MyApp custom resource

crd = k8s.obj.crd(
    group = "example.io",
    version = "v1",
    kind = "MyApp",
    plural = "myapps",
    scope = "Namespaced",
    spec = {
        "image": {"type": "string", "required": True},
        "replicas": {"type": "integer", "default": 1},
    },
    status = {
        "ready": {"type": "boolean"},
        "message": {"type": "string"},
    },
)

print(k8s.yaml(crd))

---
title: "Piping to kubectl"
description: "Generate manifests in a script and apply them with kubectl"
weight: 20
---

# Piping to kubectl

Sometimes you want starkite to build the manifest but leave the cluster mutation to `kubectl`. A workflow that has standardized on `kubectl apply` already has the RBAC, dry-run, and diff tooling it trusts; starkite earns its place by handling the templating and logic that plain YAML cannot. The bridge between the two is stdout: your script writes a manifest to standard output, and you pipe it straight into `kubectl`.

## Emit a manifest to stdout

Start by building the object the way you would any other value — as a dict — then encode it to YAML and print it. Because the dict is ordinary Starlark, you can inject variables and branch on them while you assemble it:

```python
# gen.star
manifest = {
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {"name": "web"},
    "spec": {
        "replicas": var_int("replicas", 3),
        "selector": {"matchLabels": {"app": "web"}},
        "template": {
            "metadata": {"labels": {"app": "web"}},
            "spec": {"containers": [{"name": "web", "image": var_str("image", "nginx:latest")}]},
        },
    },
}
print(yaml.encode(manifest))
```

Here `var_int` and `var_str` pull `replicas` and `image` from the command line, falling back to the defaults you supply when a flag is absent. The `print(yaml.encode(manifest))` is what reaches stdout — a single YAML document ready for a consumer.

That consumer is `kubectl`. Pass `-f -` so it reads the manifest from the pipe rather than a file:

```bash
kite run ./gen.star --var image=myapp:v2 | kubectl apply -f -
kite run ./gen.star --var image=myapp:v2 | kubectl diff -f -
```

The first line applies the rendered deployment; the second shows you what would change without touching the cluster. Swapping `apply` for `diff` costs you nothing on the starkite side, since the script only ever produces the manifest.

## Multiple objects

A real stack is rarely one object. When you have several — a deployment, a service, a configmap — encode them together with `yaml.encode_all`, which writes a multi-document stream separated the way `kubectl` expects:

```python
print(yaml.encode_all([deployment, service, configmap]))
```

The stream pipes to `kubectl` exactly as a single document does:

```bash
kite run ./stack.star | kubectl apply -f -
```

`kubectl` reads each `---`-delimited document and applies them in order, so the whole stack lands in one command.

## When to pipe instead of apply

Piping keeps the cluster mutation — and its RBAC, dry-run, and diff tooling — inside `kubectl`, while starkite contributes the variable injection and conditional logic that raw YAML lacks. The cost is an extra process and a dependency on `kubectl` being installed and configured on the host. When you would rather have the script talk to the cluster directly and skip that handoff, see [Deploying resources](deploying.md).

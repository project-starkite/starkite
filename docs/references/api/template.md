---
title: "template"
description: "Go text/template rendering for strings, files, and bytes"
weight: 14
---

The `template` module provides Go `text/template` rendering for strings, files, and byte data.

## Factories

| Factory | Returns | Description |
|---------|---------|-------------|
| `template.text(template_str, delims=())` | `template` | Create a template from a string |
| `template.file(path, delims=())` | `template` | Create a template from a file |
| `template.bytes(data, delims=())` | `template` | Create a template from raw bytes |

The optional `delims` parameter is a tuple of `(left, right)` delimiters to override the default `{{` and `}}`.

## Convenience Function

| Function | Returns | Description |
|----------|---------|-------------|
| `template.render(template_str, data)` | `string` | Render a template string with `data` in one call |

## template Object

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `render(data, missing_key="default")` | `string` | Render the template with `data`. `missing_key` controls behavior for missing keys: `"default"`, `"zero"`, `"error"` |
| `try_render(data, missing_key="default")` | `Result` | Like `render`, returns `Result` instead of raising |
| `source` | property | The raw template source string |

## Examples

### Quick render

```python
output = template.render("Hello, {{.name}}!", {"name": "world"})
print(output)  # Hello, world!
```

### Template from a string

```python
tmpl = template.text("{{.greeting}}, {{.target}}!")
output = tmpl.render({"greeting": "Hi", "target": "kite"})
print(output)  # Hi, kite!
```

### Template from a file

```python
tmpl = template.file("deploy.tmpl")
output = tmpl.render({
    "app": "myservice",
    "replicas": 3,
    "image": "myservice:v1.2.0",
})
write_text("deploy.yaml", output)
```

### Custom delimiters

```python
tmpl = template.text("<% .name %>", delims=("<%", "%>"))
output = tmpl.render({"name": "custom"})
print(output)  # custom
```

### Error handling with missing keys

```python
tmpl = template.text("{{.required_field}}")
result = tmpl.try_render({}, missing_key="error")
if not result.ok:
    print("Missing key:", result.error)
```

### Inspecting template source

```python
tmpl = template.file("template.tmpl")
print("Template:", tmpl.source)
```

> **Note:**
All `template` functions and methods that can fail support `try_` variants that return a `Result` instead of raising an error.


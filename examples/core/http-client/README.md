# HTTP Client Examples

Examples demonstrating the top-level HTTP client convenience functions in starkite.

## Overview

The `http` module provides top-level request functions (`http.get`, `http.post`, `http.put`, `http.patch`, `http.delete`) and their fault-tolerant `try_` counterparts (`http.try_get`, etc.). These allow issuing HTTP requests without constructing an intermediate `http.url` client instance.

## Examples

### requests.star

Demonstrates:
- Issuing GET, POST, PUT, and DELETE requests
- Automatic JSON encoding when passing a dictionary `body`
- Custom headers and request timeouts
- Inspecting status codes, headers, and response text via `resp.get_text()`
- Safe error handling using `http.try_get()` with the `Result` type (`res.ok`, `res.value`, `res.error`)

#### Running

Outbound HTTP requests require network access. Pass `--allow-net` (or `--permissions=allow-net`):

```bash
# Execute against live endpoints
kite run --allow-net ./requests.star

# Simulate requests without making network calls
kite run --dry-run --allow-net ./requests.star
```

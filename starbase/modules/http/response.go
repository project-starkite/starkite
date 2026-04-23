package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// writeResponse writes a Starlark handler result to an http.ResponseWriter.
//
// Response rules:
//   - None → 204 No Content
//   - string → 200 text/plain
//   - dict with "body" key → explicit response (status, headers, body)
//   - dict without "body" key → auto-serialize as JSON, application/json
func writeResponse(w http.ResponseWriter, result starlark.Value) error {
	// None → 204 No Content
	if result == nil || result == starlark.None {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	// String → 200 text/plain
	if s, ok := starlark.AsString(result); ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(s))
		return err
	}

	// Dict
	d, ok := result.(*starlark.Dict)
	if !ok {
		return fmt.Errorf("handler must return dict, string, or None; got %s", result.Type())
	}

	// Check if dict has "body" key → explicit response
	bodyVal, found, err := d.Get(starlark.String("body"))
	if err != nil {
		return fmt.Errorf("reading body key: %w", err)
	}

	if found && bodyVal != nil {
		return writeExplicitResponse(w, d, bodyVal)
	}

	// Dict without "body" → auto-serialize as JSON
	return writeAutoJSON(w, d)
}

// writeExplicitResponse handles dicts with a "body" key: {status, headers, body}.
func writeExplicitResponse(w http.ResponseWriter, d *starlark.Dict, bodyVal starlark.Value) error {
	// Extract status (default 200)
	status := http.StatusOK
	if statusVal, found, _ := d.Get(starlark.String("status")); found && statusVal != nil {
		if code, err := starlark.AsInt32(statusVal); err == nil {
			status = int(code)
		}
	}

	// Extract and set headers
	if headersVal, found, _ := d.Get(starlark.String("headers")); found && headersVal != nil {
		if hd, ok := headersVal.(*starlark.Dict); ok {
			for _, item := range hd.Items() {
				if k, ok := starlark.AsString(item[0]); ok {
					if v, ok := starlark.AsString(item[1]); ok {
						w.Header().Set(k, v)
					}
				}
			}
		}
	}

	// Write body
	body, ok := starlark.AsString(bodyVal)
	if !ok {
		body = bodyVal.String()
	}

	// Set Content-Type if not already set
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	w.WriteHeader(status)
	_, err := w.Write([]byte(body))
	return err
}

// writeAutoJSON serializes the entire dict as JSON.
// Uses a typed map[string]any target so startype produces JSON-compatible output.
func writeAutoJSON(w http.ResponseWriter, d *starlark.Dict) error {
	var goVal map[string]any
	if err := startype.Starlark(d).Go(&goVal); err != nil {
		return fmt.Errorf("json serialization: %w", err)
	}

	// startype produces map[any]any for nested dicts when the value target is
	// interface{}. Sanitize so json.Marshal can handle it.
	sanitizeMapKeys(goVal)

	data, err := json.Marshal(goVal)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, writeErr := w.Write(data)
	return writeErr
}

// sanitizeMapKeys recursively converts map[any]any (produced by startype for
// nested dicts) to map[string]any so the result is JSON-serializable.
func sanitizeMapKeys(m map[string]any) {
	for k, v := range m {
		m[k] = sanitizeValue(v)
	}
}

func sanitizeValue(v any) any {
	switch val := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(val))
		for mk, mv := range val {
			out[fmt.Sprintf("%v", mk)] = sanitizeValue(mv)
		}
		return out
	case map[string]any:
		sanitizeMapKeys(val)
		return val
	case []any:
		for i, elem := range val {
			val[i] = sanitizeValue(elem)
		}
		return val
	default:
		return v
	}
}

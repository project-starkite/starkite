package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// writeResponse writes a Starlark handler result to an http.ResponseWriter.
//
// Response rules:
//   - None → 204 No Content
//   - string → 200 text/plain
//   - dict/struct with "body" attribute/key → explicit response (status, headers, body)
//   - dict/struct without "body" → auto-serialize as JSON, application/json
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

	d, ok, err := libkite.ValueToDict(result)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("handler must return dict, struct, string, or None; got %s", result.Type())
	}

	// Check if mapping has "body" key → explicit response
	bodyVal, found, err := d.Get(starlark.String("body"))
	if err != nil {
		return fmt.Errorf("reading body key: %w", err)
	}

	if found && bodyVal != nil {
		return writeExplicitResponse(w, d, bodyVal)
	}

	// Dict/struct without "body" → auto-serialize as JSON
	return writeAutoJSON(w, d)
}

// writeExplicitResponse handles dicts/structs with a "body" field: {status, headers, body}.
func writeExplicitResponse(w http.ResponseWriter, d *starlark.Dict, bodyVal starlark.Value) error {
	// Extract status (default 200)
	status := http.StatusOK
	if statusVal, found, _ := d.Get(starlark.String("status")); found && statusVal != nil && statusVal != starlark.None {
		code, err := starlark.AsInt32(statusVal)
		if err != nil {
			return fmt.Errorf("http.server: status must be an integer, got %s", statusVal.Type())
		}
		if code < 100 || code > 999 {
			return fmt.Errorf("http.server: invalid HTTP status code %d (must be 100-999)", code)
		}
		status = int(code)
	}

	// Extract and set headers
	if headersVal, found, _ := d.Get(starlark.String("headers")); found && headersVal != nil && headersVal != starlark.None {
		hd, ok, err := libkite.ValueToDict(headersVal)
		if err != nil {
			return fmt.Errorf("http.server: headers: %w", err)
		}
		if ok {
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

// writeAutoJSON serializes a dict or struct as JSON.
func writeAutoJSON(w http.ResponseWriter, d *starlark.Dict) error {
	goVal, _, err := libkite.ValueToMap(d)
	if err != nil {
		return fmt.Errorf("json serialization: %w", err)
	}

	data, err := json.Marshal(goVal)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, writeErr := w.Write(data)
	return writeErr
}

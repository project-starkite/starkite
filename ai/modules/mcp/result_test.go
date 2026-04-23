package mcp

import (
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"
)

func TestMCPResult_TextConcatenation(t *testing.T) {
	r := newMCPResult(&mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "hello "},
			&mcpsdk.TextContent{Text: "world"},
		},
	})
	v, _ := r.Attr("text")
	s, _ := starlark.AsString(v)
	if s != "hello world" {
		t.Errorf("text = %q, want 'hello world'", s)
	}
}

func TestMCPResult_IsError(t *testing.T) {
	r := newMCPResult(&mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "boom"}},
	})
	v, _ := r.Attr("is_error")
	b, ok := v.(starlark.Bool)
	if !ok || !bool(b) {
		t.Errorf("is_error = %v, want True", v)
	}
}

func TestMCPResult_Content_IncludesImageEntry(t *testing.T) {
	r := newMCPResult(&mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "desc"},
			&mcpsdk.ImageContent{Data: []byte{1, 2, 3}, MIMEType: "image/png"},
		},
	})
	v, _ := r.Attr("content")
	list, ok := v.(*starlark.List)
	if !ok {
		t.Fatalf("content is %T, want *starlark.List", v)
	}
	if list.Len() != 2 {
		t.Fatalf("content len = %d, want 2", list.Len())
	}
	// Image entry
	imgEntry, _ := list.Index(1).(*starlark.Dict)
	typeVal, _, _ := imgEntry.Get(starlark.String("type"))
	if s, _ := starlark.AsString(typeVal); s != "image" {
		t.Errorf("image entry type = %q", s)
	}
	mimeVal, _, _ := imgEntry.Get(starlark.String("mime_type"))
	if s, _ := starlark.AsString(mimeVal); s != "image/png" {
		t.Errorf("image mime_type = %q", s)
	}
	dataVal, _, _ := imgEntry.Get(starlark.String("data"))
	if _, ok := dataVal.(starlark.Bytes); !ok {
		t.Errorf("image data type = %T, want starlark.Bytes", dataVal)
	}
}

func TestMCPResult_AttrNames(t *testing.T) {
	r := newMCPResult(&mcpsdk.CallToolResult{})
	names := r.AttrNames()
	want := map[string]bool{"text": true, "content": true, "is_error": true}
	for _, n := range names {
		delete(want, n)
	}
	if len(want) > 0 {
		t.Errorf("missing attrs: %v", want)
	}
}

func TestMCPResult_TextEmpty_WhenNoTextParts(t *testing.T) {
	r := newMCPResult(&mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.ImageContent{MIMEType: "image/png"},
		},
	})
	v, _ := r.Attr("text")
	if s, _ := starlark.AsString(v); s != "" {
		t.Errorf("text = %q, want empty", s)
	}
}

func TestMCPResult_Truth_FalseOnError(t *testing.T) {
	r := newMCPResult(&mcpsdk.CallToolResult{IsError: true})
	if bool(r.Truth()) {
		t.Errorf("Truth = True, want False on error")
	}
	r2 := newMCPResult(&mcpsdk.CallToolResult{})
	if !bool(r2.Truth()) {
		t.Errorf("Truth = False, want True on success")
	}
}

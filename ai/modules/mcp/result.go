package mcp

import (
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"
)

// MCPResult is the Starlark-visible result of an MCP tool call made via
// client.tools.<name>(...) or client.call(...).
//
// Attributes exposed to scripts:
//
//	.text      — concatenation of every TextContent part (may be "")
//	.content   — list of dicts, one per Content part (with per-type fields)
//	.is_error  — bool; true when the server returned IsError=true
type MCPResult struct {
	text    string
	content *starlark.List
	isError bool
}

var _ starlark.HasAttrs = (*MCPResult)(nil)

// newMCPResult converts an SDK CallToolResult into its Starlark projection.
// Walks the Content slice once: each item becomes a dict entry on .content,
// and any TextContent.Text is accumulated into a single .text string.
func newMCPResult(res *mcpsdk.CallToolResult) *MCPResult {
	var texts []string
	items := make([]starlark.Value, 0, len(res.Content))
	for _, c := range res.Content {
		items = append(items, contentToStarlarkDict(c, &texts))
	}
	list := starlark.NewList(items)
	list.Freeze()
	return &MCPResult{
		text:    strings.Join(texts, ""),
		content: list,
		isError: res.IsError,
	}
}

func (r *MCPResult) String() string {
	if r.isError {
		return fmt.Sprintf("<mcp.Result error=%q>", r.text)
	}
	return fmt.Sprintf("<mcp.Result text=%q>", r.text)
}
func (r *MCPResult) Type() string          { return "mcp.Result" }
func (r *MCPResult) Freeze()               {}
func (r *MCPResult) Truth() starlark.Bool  { return starlark.Bool(!r.isError) }
func (r *MCPResult) Hash() (uint32, error) { return 0, fmt.Errorf("mcp.Result is unhashable") }

func (r *MCPResult) Attr(name string) (starlark.Value, error) {
	switch name {
	case "text":
		return starlark.String(r.text), nil
	case "content":
		return r.content, nil
	case "is_error":
		return starlark.Bool(r.isError), nil
	}
	return nil, nil
}

func (r *MCPResult) AttrNames() []string { return []string{"text", "content", "is_error"} }

// contentToStarlarkDict projects a single MCP Content item into a Starlark
// dict. Text parts also append to texts so the caller can build .text by
// joining. Unknown content types degrade to a dict with just a "type" key.
func contentToStarlarkDict(c mcpsdk.Content, texts *[]string) starlark.Value {
	d := starlark.NewDict(4)
	switch v := c.(type) {
	case *mcpsdk.TextContent:
		_ = d.SetKey(starlark.String("type"), starlark.String("text"))
		_ = d.SetKey(starlark.String("text"), starlark.String(v.Text))
		*texts = append(*texts, v.Text)
	case *mcpsdk.ImageContent:
		_ = d.SetKey(starlark.String("type"), starlark.String("image"))
		_ = d.SetKey(starlark.String("data"), starlark.Bytes(v.Data))
		_ = d.SetKey(starlark.String("mime_type"), starlark.String(v.MIMEType))
	case *mcpsdk.AudioContent:
		_ = d.SetKey(starlark.String("type"), starlark.String("audio"))
		_ = d.SetKey(starlark.String("data"), starlark.Bytes(v.Data))
		_ = d.SetKey(starlark.String("mime_type"), starlark.String(v.MIMEType))
	default:
		// Unknown content type: still surface the type name so the user can
		// see what arrived. Better than dropping the entry entirely.
		_ = d.SetKey(starlark.String("type"), starlark.String(fmt.Sprintf("%T", v)))
	}
	d.Freeze()
	return d
}

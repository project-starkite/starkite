package genai

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Response is the Starlark-visible return value of ai.generate().
//
// Attributes:
//
//	.text   — generated text (raw JSON when schema= was requested)
//	.model  — model identifier used for the call
//	.usage  — Usage object with .input, .output, .total token counts
//	.data   — parsed structured output (only when schema= was requested); else None
type Response struct {
	text  string
	model string
	usage *Usage
	data  starlark.Value // nil when schema was not requested; caller uses Attr to map to None
}

var _ starlark.HasAttrs = (*Response)(nil)

// newResponse constructs a Response from the values our client layer produces.
// data may be nil (no schema requested) or any starlark.Value.
func newResponse(text, model string, inputTokens, outputTokens int, data starlark.Value) *Response {
	return &Response{
		text:  text,
		model: model,
		usage: &Usage{
			input:  inputTokens,
			output: outputTokens,
		},
		data: data,
	}
}

func (r *Response) String() string        { return r.text }
func (r *Response) Type() string          { return "ai.Response" }
func (r *Response) Freeze()               {}
func (r *Response) Truth() starlark.Bool  { return starlark.Bool(r.text != "") }
func (r *Response) Hash() (uint32, error) { return 0, fmt.Errorf("ai.Response is unhashable") }

func (r *Response) Attr(name string) (starlark.Value, error) {
	switch name {
	case "text":
		return starlark.String(r.text), nil
	case "model":
		return starlark.String(r.model), nil
	case "usage":
		return r.usage, nil
	case "data":
		if r.data == nil {
			return starlark.None, nil
		}
		return r.data, nil
	}
	return nil, nil
}

func (r *Response) AttrNames() []string { return []string{"text", "model", "usage", "data"} }

// Usage exposes token counts. .total is derived (.input + .output) to match
// common expectations even though Genkit doesn't report total directly.
type Usage struct {
	input  int
	output int
}

var _ starlark.HasAttrs = (*Usage)(nil)

func (u *Usage) String() string {
	return fmt.Sprintf("ai.Usage(input=%d, output=%d)", u.input, u.output)
}
func (u *Usage) Type() string          { return "ai.Usage" }
func (u *Usage) Freeze()               {}
func (u *Usage) Truth() starlark.Bool  { return starlark.Bool(u.input > 0 || u.output > 0) }
func (u *Usage) Hash() (uint32, error) { return 0, fmt.Errorf("ai.Usage is unhashable") }

func (u *Usage) Attr(name string) (starlark.Value, error) {
	switch name {
	case "input":
		return starlark.MakeInt(u.input), nil
	case "output":
		return starlark.MakeInt(u.output), nil
	case "total":
		return starlark.MakeInt(u.input + u.output), nil
	}
	return nil, nil
}

func (u *Usage) AttrNames() []string { return []string{"input", "output", "total"} }

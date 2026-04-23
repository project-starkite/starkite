package genai

import (
	"fmt"
	"sync/atomic"

	"go.starlark.net/starlark"
)

// StreamValue is the Starlark-visible handle to a streaming generation.
// It is iterable (yields *StreamChunk values with .text) and has attributes:
//
//	.error  — "" if the stream completed cleanly; error message otherwise
//	.model  — model identifier used
//	.usage  — Usage (only meaningful after iteration completes)
//
// Starlark iterators cannot raise errors mid-iteration. If generation fails
// after some chunks have been delivered, iteration stops cleanly and .error
// is populated. Callers should check .error after consuming the stream:
//
//	stream = ai.generate("hi", stream=True, model="openai/x")
//	for chunk in stream:
//	    printf(chunk.text)
//	if stream.error:
//	    fail(stream.error)
type StreamValue struct {
	result    *StreamResult
	consumed  atomic.Bool
	fullUsage *Usage // memoized Usage, rebuilt from result after drain
}

var (
	_ starlark.Value    = (*StreamValue)(nil)
	_ starlark.Iterable = (*StreamValue)(nil)
	_ starlark.HasAttrs = (*StreamValue)(nil)
	_ starlark.Iterator = (*streamIterator)(nil)
	_ starlark.HasAttrs = (*StreamChunk)(nil)
)

func newStreamValue(r *StreamResult) *StreamValue { return &StreamValue{result: r} }

func (s *StreamValue) String() string       { return "<ai.Stream>" }
func (s *StreamValue) Type() string         { return "ai.Stream" }
func (s *StreamValue) Freeze()              {}
func (s *StreamValue) Truth() starlark.Bool { return starlark.Bool(s.result != nil) }
func (s *StreamValue) Hash() (uint32, error) {
	return 0, fmt.Errorf("ai.Stream is unhashable")
}

// Iterate returns a one-shot iterator over the remaining chunks. Calling
// Iterate a second time returns an iterator that yields nothing.
func (s *StreamValue) Iterate() starlark.Iterator {
	if s.consumed.Swap(true) {
		return &streamIterator{exhausted: true}
	}
	return &streamIterator{sv: s}
}

func (s *StreamValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "error":
		if s.result == nil {
			return starlark.String(""), nil
		}
		if err := s.result.Err(); err != nil {
			return starlark.String(err.Error()), nil
		}
		return starlark.String(""), nil
	case "model":
		if s.result == nil {
			return starlark.String(""), nil
		}
		return starlark.String(s.result.Model()), nil
	case "usage":
		if s.fullUsage != nil {
			return s.fullUsage, nil
		}
		if s.result == nil {
			return &Usage{}, nil
		}
		in, out := s.result.Usage()
		s.fullUsage = &Usage{input: in, output: out}
		return s.fullUsage, nil
	}
	return nil, nil
}

func (s *StreamValue) AttrNames() []string { return []string{"error", "model", "usage"} }

// streamIterator yields *StreamChunk values from StreamValue.result.Chunks.
// exhausted is true for the second-iterate case, where we yield nothing.
type streamIterator struct {
	sv        *StreamValue
	exhausted bool
}

func (it *streamIterator) Next(p *starlark.Value) bool {
	if it.exhausted || it.sv == nil || it.sv.result == nil {
		return false
	}
	text, ok := <-it.sv.result.Chunks
	if !ok {
		it.exhausted = true
		return false
	}
	*p = &StreamChunk{text: text}
	return true
}

// Done signals early termination. If the consumer broke out of iteration,
// we cancel the producer goroutine so it doesn't block on a full channel.
func (it *streamIterator) Done() {
	if it.exhausted || it.sv == nil || it.sv.result == nil {
		return
	}
	it.sv.result.Cancel()
	// Drain remaining chunks so the producer can exit cleanly.
	for range it.sv.result.Chunks {
	}
	it.exhausted = true
}

// StreamChunk is the per-iteration value a script sees during streaming.
// Attribute: .text — the text delta for this chunk.
type StreamChunk struct {
	text string
}

func (c *StreamChunk) String() string        { return c.text }
func (c *StreamChunk) Type() string          { return "ai.StreamChunk" }
func (c *StreamChunk) Freeze()               {}
func (c *StreamChunk) Truth() starlark.Bool  { return starlark.Bool(c.text != "") }
func (c *StreamChunk) Hash() (uint32, error) { return 0, fmt.Errorf("ai.StreamChunk is unhashable") }

func (c *StreamChunk) Attr(name string) (starlark.Value, error) {
	if name == "text" {
		return starlark.String(c.text), nil
	}
	return nil, nil
}

func (c *StreamChunk) AttrNames() []string { return []string{"text"} }

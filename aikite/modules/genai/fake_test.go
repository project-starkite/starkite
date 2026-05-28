package genai

import (
	"context"
	"fmt"
)

// fakeClient is a test double for genaiClient.
//
// Configure it per-test:
//   - .script sets canned Generate responses in order.
//   - .err, if non-nil, is returned from Generate/GenerateStream instead of consuming.
//   - .streamScript sets canned stream responses in order.
//   - .calls records every incoming request for assertions.
type fakeClient struct {
	script       []*GenerateResult
	err          error
	streamScript []*fakeStream
	calls        []GenerateRequest

	// preGenerateHook, if set, is invoked at the start of Generate with the
	// incoming request. Tests use it to populate req.ToolTrace (mimicking
	// tool callbacks) so chat trace-persistence behavior can be verified
	// without standing up a real Genkit loop.
	preGenerateHook func(req GenerateRequest)
}

// fakeStream describes one scripted stream: chunks delivered in order, then
// optional terminal error.
type fakeStream struct {
	chunks       []string
	err          error
	model        string
	inputTokens  int
	outputTokens int
}

func (f *fakeClient) Generate(_ context.Context, req GenerateRequest) (*GenerateResult, error) {
	if f.preGenerateHook != nil {
		f.preGenerateHook(req)
	}
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.script) == 0 {
		return nil, fmt.Errorf("fakeClient: script exhausted (%d prior calls)", len(f.calls))
	}
	next := f.script[0]
	f.script = f.script[1:]
	return next, nil
}

func (f *fakeClient) GenerateStream(_ context.Context, req GenerateRequest) (*StreamResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.streamScript) == 0 {
		return nil, fmt.Errorf("fakeClient: streamScript exhausted (%d prior calls)", len(f.calls))
	}
	s := f.streamScript[0]
	f.streamScript = f.streamScript[1:]

	// Pre-populate and close the channel synchronously so tests don't need
	// to manage goroutine timing. The real client uses a background goroutine.
	ch := make(chan string, len(s.chunks))
	for _, c := range s.chunks {
		ch <- c
	}
	close(ch)

	sr := &StreamResult{
		Chunks:       ch,
		model:        s.model,
		err:          s.err,
		inputTokens:  s.inputTokens,
		outputTokens: s.outputTokens,
		cancel:       func() {}, // no-op for fake
	}
	return sr, nil
}

// installFake wires a ready-to-go fakeClient into a Module, returning a
// pointer to the fake for per-test configuration. The Module's construction
// of production clients is bypassed.
func installFake(m *Module) *fakeClient {
	f := &fakeClient{}
	m.newClientFunc = func(cv configView) genaiClient { return f }
	m.resetClient() // ensure next clientFor() uses the new factory
	return f
}

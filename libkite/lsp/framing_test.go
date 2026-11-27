package lsp

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
)

func lspFrame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func TestBoundedReader_ValidFrames(t *testing.T) {
	in := lspFrame(`{"id":1}`) + lspFrame(`{"id":2,"method":"shutdown"}`)
	r := NewBoundedReader(strings.NewReader(in), FramingLimits{})
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected error reading valid frames: %v", err)
	}
	if string(got) != in {
		t.Fatalf("frame mismatch: got %q, want %q", string(got), in)
	}
}

func TestBoundedReader_SmallReadBuffer(t *testing.T) {
	in := lspFrame(`{"jsonrpc":"2.0","id":1}`)
	r := NewBoundedReader(strings.NewReader(in), FramingLimits{})
	var out bytes.Buffer
	buf := make([]byte, 3)
	for {
		n, err := r.Read(buf)
		out.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if out.String() != in {
		t.Fatalf("got %q, want %q", out.String(), in)
	}
}

func TestBoundedReader_RejectsOversizedContentLength(t *testing.T) {
	frame := "Content-Length: 99999999\r\n\r\n{}"
	r := NewBoundedReader(strings.NewReader(frame), FramingLimits{MaxContentLength: 1024})
	_, err := io.ReadAll(r)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected oversized Content-Length error, got %v", err)
	}
}

func TestBoundedReader_RejectsNegativeAndMalformedContentLength(t *testing.T) {
	for _, hdr := range []string{"Content-Length: -1\r\n\r\n", "Content-Length: nope\r\n\r\n"} {
		r := NewBoundedReader(strings.NewReader(hdr), FramingLimits{})
		if _, err := io.ReadAll(r); err == nil {
			t.Fatalf("expected error for header %q", hdr)
		}
	}
}

func TestBoundedReader_RejectsOversizedHeaderLine(t *testing.T) {
	longVal := strings.Repeat("a", 128)
	frame := "X-Test: " + longVal + "\r\nContent-Length: 2\r\n\r\n{}"
	r := NewBoundedReader(strings.NewReader(frame), FramingLimits{MaxHeaderLineBytes: 64})
	_, err := io.ReadAll(r)
	if err == nil || !strings.Contains(err.Error(), "header line exceeds") {
		t.Fatalf("expected oversized header line error, got %v", err)
	}
}

func TestBoundedReader_RejectsTooManyHeaders(t *testing.T) {
	var sb strings.Builder
	for i := range 5 {
		fmt.Fprintf(&sb, "X-H-%d: v\r\n", i)
	}
	sb.WriteString("Content-Length: 2\r\n\r\n{}")
	r := NewBoundedReader(strings.NewReader(sb.String()), FramingLimits{MaxHeaders: 3})
	_, err := io.ReadAll(r)
	if err == nil || !strings.Contains(err.Error(), "header count exceeds") {
		t.Fatalf("expected too many headers error, got %v", err)
	}
}

func TestHost_PanicRecovery(t *testing.T) {
	var logs bytes.Buffer
	h := NewHost(func(*libkite.ModuleConfig) *libkite.Registry {
		panic("boom")
	})
	h.SetLog(&logs)

	syms := h.Globals()
	if syms != nil {
		t.Fatalf("expected nil symbols on recovered panic, got %v", syms)
	}
	if !strings.Contains(logs.String(), "starkite lsp host panic in Globals: boom") {
		t.Fatalf("expected panic log output, got %q", logs.String())
	}
}

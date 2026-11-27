package iomod

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

type trackingReader struct {
	io.Reader
	closed bool
}

func (tr *trackingReader) Close() error {
	tr.closed = true
	return nil
}

func TestIOStreams(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		setup      func() (starlark.Tuple, any)
		wantResult string
		wantErr    string
		verify     func(t *testing.T, ctx any)
	}{
		{
			name: "reader read bytes",
			script: `
def test(r):
    res = r.read(5)
    return str(res)
`,
			setup: func() (starlark.Tuple, any) {
				r := NewReader(strings.NewReader("hello world"), "test_input")
				return starlark.Tuple{r}, nil
			},
			wantResult: "hello",
		},
		{
			name: "reader bytes",
			script: `
def test(r):
    res = r.bytes()
    return str(res)
`,
			setup: func() (starlark.Tuple, any) {
				r := NewReader(strings.NewReader("hello world"), "test_input")
				return starlark.Tuple{r}, nil
			},
			wantResult: "hello world",
		},
		{
			name: "reader lines iterator",
			script: `
def test(r):
    lines = []
    for line in r.lines():
        lines.append(line.strip())
    return ", ".join(lines)
`,
			setup: func() (starlark.Tuple, any) {
				r := NewReader(strings.NewReader("line1\nline2\nline3"), "test_input")
				return starlark.Tuple{r}, nil
			},
			wantResult: "line1, line2, line3",
		},
		{
			name: "writer write string",
			script: `
def test(w):
    w.write("hello")
    w.write(" ")
    w.write("world")
    w.close()
    return "ok"
`,
			setup: func() (starlark.Tuple, any) {
				var buf bytes.Buffer
				w := NewWriter(&buf, "test_output")
				return starlark.Tuple{w}, &buf
			},
			wantResult: "ok",
			verify: func(t *testing.T, ctx any) {
				buf := ctx.(*bytes.Buffer)
				if got := buf.String(); got != "hello world" {
					t.Errorf("writer output = %q, want %q", got, "hello world")
				}
			},
		},
		{
			name: "writer pipe reader and auto close",
			script: `
def test(w, r):
    n = w.write(r)
    w.close()
    return str(n)
`,
			setup: func() (starlark.Tuple, any) {
				var buf bytes.Buffer
				w := NewWriter(&buf, "test_output")
				tr := &trackingReader{Reader: strings.NewReader("piped data")}
				r := NewReader(tr, "test_input")
				return starlark.Tuple{w, r}, tr
			},
			wantResult: "10",
			verify: func(t *testing.T, ctx any) {
				tr := ctx.(*trackingReader)
				if !tr.closed {
					t.Error("expected source reader to be automatically closed upon piping completion")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := libkite.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Close()

			ioMod := New()
			dict, err := ioMod.Load(nil)
			if err != nil {
				t.Fatal(err)
			}

			predeclared := starlark.StringDict{
				"io": dict["io"],
			}

			thread := rt.NewThread("test-thread")
			globals, err := starlark.ExecFile(thread, "test.star", tc.script, predeclared)
			if err != nil {
				t.Fatal(err)
			}

			testFn, ok := globals["test"]
			if !ok {
				t.Fatal("test function not found")
			}

			var args starlark.Tuple
			var ctx any
			if tc.setup != nil {
				args, ctx = tc.setup()
			}

			res, err := starlark.Call(thread, testFn, args, nil)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %q, want it to contain %q", err.Error(), tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if tc.wantResult != "" {
					gotStr, ok := starlark.AsString(res)
					if !ok {
						t.Fatalf("expected string result, got %s", res.Type())
					}
					if gotStr != tc.wantResult {
						t.Errorf("got %q, want %q", gotStr, tc.wantResult)
					}
				}

				if tc.verify != nil {
					tc.verify(t, ctx)
				}
			}
		})
	}
}

type failingReaderCloser struct {
	err    error
	closed bool
}

func (f *failingReaderCloser) Read([]byte) (int, error) { return 0, f.err }
func (f *failingReaderCloser) Close() error             { f.closed = true; return nil }

type flushCloseErrorWriter struct {
	flushErr error
	closeErr error
}

func (w *flushCloseErrorWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *flushCloseErrorWriter) Flush() error                { return w.flushErr }
func (w *flushCloseErrorWriter) Close() error                { return w.closeErr }

func TestReaderLinesCloseAndIteratorErrAndWriterFlushErr(t *testing.T) {
	// 1. lines() must allow explicit reader.close() afterward and close the underlying closer.
	tr := &trackingReader{Reader: strings.NewReader("alpha\nbeta\n")}
	r := NewReader(tr, "lines").(*starlarkReader)
	thread := &starlark.Thread{Name: "test"}
	linesFn, err := r.Attr("lines")
	if err != nil {
		t.Fatal(err)
	}
	itVal, err := starlark.Call(thread, linesFn, nil, nil)
	if err != nil {
		t.Fatalf("lines() failed: %v", err)
	}
	it := itVal.(starlark.Iterable).Iterate()
	var item starlark.Value
	if !it.Next(&item) || string(item.(starlark.String)) != "alpha\n" {
		t.Fatalf("unexpected first line: %v", item)
	}
	it.Done()
	closeFn, err := r.Attr("close")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := starlark.Call(thread, closeFn, nil, nil); err != nil {
		t.Fatalf("reader.close() after lines() failed: %v", err)
	}
	if !tr.closed {
		t.Fatal("expected underlying reader to be closed")
	}

	// 2. Non-EOF read errors must be recorded on starlarkLineIterator.Err().
	fr := &failingReaderCloser{err: io.ErrUnexpectedEOF}
	rErr := NewReader(fr, "err").(*starlarkReader)
	linesErrFn, _ := rErr.Attr("lines")
	itErrVal, err := starlark.Call(thread, linesErrFn, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	lineIter := itErrVal.(*starlarkLineIterator)
	if lineIter.Next(&item) {
		t.Fatal("expected Next() to return false on read error")
	}
	if lineIter.Err() != io.ErrUnexpectedEOF {
		t.Fatalf("lineIter.Err() = %v, want %v", lineIter.Err(), io.ErrUnexpectedEOF)
	}
	if !fr.closed {
		t.Fatal("expected failing reader to be closed on error")
	}

	// 3. writer.Close() must propagate Flush() errors alongside Close() errors.
	fw := &flushCloseErrorWriter{flushErr: io.ErrShortWrite, closeErr: io.ErrClosedPipe}
	sw := NewWriter(fw, "flusherr").(*starlarkWriter)
	if err := sw.Close(); err == nil || !strings.Contains(err.Error(), io.ErrShortWrite.Error()) || !strings.Contains(err.Error(), io.ErrClosedPipe.Error()) {
		t.Fatalf("expected joined flush + close error, got %v", err)
	}
}

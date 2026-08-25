package iomod

import (
	"bufio"
	"fmt"
	"io"
	"runtime"
	"sync"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// starlarkReader wraps a Go io.Reader as a Starlark value.
type starlarkReader struct {
	reader io.Reader
	closer io.Closer
	closed bool
	mu     sync.Mutex
	name   string
}

var (
	_ starlark.Value         = (*starlarkReader)(nil)
	_ starlark.HasAttrs      = (*starlarkReader)(nil)
	_ libkite.StarlarkReader = (*starlarkReader)(nil)
	_ io.Closer              = (*starlarkReader)(nil)
)

// NewReader wraps r as an io.reader Starlark value.
func NewReader(r io.Reader, name string) starlark.Value {
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}
	return NewReaderWithCloser(r, closer, name)
}

// NewReaderWithCloser wraps r as an io.reader Starlark value with an explicit closer.
func NewReaderWithCloser(r io.Reader, closer io.Closer, name string) starlark.Value {
	sr := &starlarkReader{
		reader: r,
		closer: closer,
		name:   name,
	}
	runtime.SetFinalizer(sr, func(x *starlarkReader) {
		x.Close()
	})
	return sr
}

func (r *starlarkReader) String() string        { return fmt.Sprintf("<io.reader name=%q>", r.name) }
func (r *starlarkReader) Type() string          { return "io.reader" }
func (r *starlarkReader) Freeze()               {}
func (r *starlarkReader) Truth() starlark.Bool  { return starlark.True }
func (r *starlarkReader) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: io.reader") }

func (r *starlarkReader) Reader() io.Reader { return r.reader }

func (r *starlarkReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

func (r *starlarkReader) Attr(name string) (starlark.Value, error) {
	switch name {
	case "read":
		return starlark.NewBuiltin("io.reader.read", r.readCmd), nil
	case "bytes":
		return starlark.NewBuiltin("io.reader.bytes", r.bytesCmd), nil
	case "lines":
		return starlark.NewBuiltin("io.reader.lines", r.linesCmd), nil
	case "close":
		return starlark.NewBuiltin("io.reader.close", r.closeCmd), nil
	}
	return nil, nil
}

func (r *starlarkReader) AttrNames() []string {
	return []string{"read", "bytes", "lines", "close"}
}

func (r *starlarkReader) readCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var count int
	if err := starlark.UnpackArgs("read", args, kwargs, "count", &count); err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, fmt.Errorf("io.reader.read: count cannot be negative")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("io.reader.read: reader is closed")
	}

	buf := make([]byte, count)
	n, err := r.reader.Read(buf)
	if n > 0 {
		return starlark.Bytes(buf[:n]), nil
	}
	if err != nil {
		if err == io.EOF {
			return starlark.Bytes(""), nil
		}
		return nil, fmt.Errorf("io.reader.read: %w", err)
	}

	return starlark.Bytes(""), nil
}

func (r *starlarkReader) bytesCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("bytes", args, kwargs); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("io.reader.bytes: reader is closed")
	}

	data, err := io.ReadAll(r.reader)
	if r.closer != nil {
		r.closer.Close()
	}
	r.closed = true

	if err != nil {
		return nil, fmt.Errorf("io.reader.bytes: %w", err)
	}

	return starlark.Bytes(data), nil
}

func (r *starlarkReader) linesCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("lines", args, kwargs); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("io.reader.lines: reader is closed")
	}

	bufReader := bufio.NewReader(r.reader)
	it := &starlarkLineIterator{
		reader: bufReader,
		closer: r.closer,
	}
	r.closed = true

	return it, nil
}

// starlarkLineIterator implements starlark.Iterator for streaming lines.
type starlarkLineIterator struct {
	reader *bufio.Reader
	closer io.Closer
	mu     sync.Mutex
	done   bool
}

var (
	_ starlark.Value    = (*starlarkLineIterator)(nil)
	_ starlark.Iterable = (*starlarkLineIterator)(nil)
	_ starlark.Iterator = (*starlarkLineIterator)(nil)
)

func (it *starlarkLineIterator) String() string       { return "<io.line_iterator>" }
func (it *starlarkLineIterator) Type() string         { return "io.line_iterator" }
func (it *starlarkLineIterator) Freeze()              {}
func (it *starlarkLineIterator) Truth() starlark.Bool { return starlark.True }
func (it *starlarkLineIterator) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: io.line_iterator")
}

func (it *starlarkLineIterator) Iterate() starlark.Iterator { return it }

func (it *starlarkLineIterator) Next(p *starlark.Value) bool {
	it.mu.Lock()
	defer it.mu.Unlock()

	if it.done {
		return false
	}

	line, err := it.reader.ReadString('\n')
	if err != nil {
		it.done = true
		if err == io.EOF {
			if line != "" {
				*p = starlark.String(line)
				return true
			}
			if it.closer != nil {
				it.closer.Close()
			}
			return false
		}
		if it.closer != nil {
			it.closer.Close()
		}
		return false
	}
	*p = starlark.String(line)
	return true
}

func (it *starlarkLineIterator) Done() {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.done = true
	if it.closer != nil {
		it.closer.Close()
	}
}

func (r *starlarkReader) closeCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("close", args, kwargs); err != nil {
		return nil, err
	}

	if err := r.Close(); err != nil {
		return nil, fmt.Errorf("io.reader.close: %w", err)
	}
	return starlark.None, nil
}

// starlarkWriter wraps a Go io.Writer as a Starlark value.
type starlarkWriter struct {
	writer io.Writer
	closer io.Closer
	closed bool
	mu     sync.Mutex
	name   string
}

var (
	_ starlark.Value         = (*starlarkWriter)(nil)
	_ starlark.HasAttrs      = (*starlarkWriter)(nil)
	_ libkite.StarlarkWriter = (*starlarkWriter)(nil)
	_ io.Closer              = (*starlarkWriter)(nil)
)

// NewWriter wraps w as an io.writer Starlark value.
func NewWriter(w io.Writer, name string) starlark.Value {
	var closer io.Closer
	if c, ok := w.(io.Closer); ok {
		closer = c
	}
	return NewWriterWithCloser(w, closer, name)
}

// NewWriterWithCloser wraps w as an io.writer Starlark value with an explicit closer.
func NewWriterWithCloser(w io.Writer, closer io.Closer, name string) starlark.Value {
	sw := &starlarkWriter{
		writer: w,
		closer: closer,
		name:   name,
	}
	runtime.SetFinalizer(sw, func(x *starlarkWriter) {
		x.Close()
	})
	return sw
}

func (w *starlarkWriter) String() string        { return fmt.Sprintf("<io.writer name=%q>", w.name) }
func (w *starlarkWriter) Type() string          { return "io.writer" }
func (w *starlarkWriter) Freeze()               {}
func (w *starlarkWriter) Truth() starlark.Bool  { return starlark.True }
func (w *starlarkWriter) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: io.writer") }

func (w *starlarkWriter) Writer() io.Writer { return w.writer }

func (w *starlarkWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true

	// Flush if supported
	if flusher, ok := w.writer.(interface{ Flush() error }); ok {
		flusher.Flush()
	}

	if w.closer != nil {
		return w.closer.Close()
	}
	return nil
}

func (w *starlarkWriter) Attr(name string) (starlark.Value, error) {
	switch name {
	case "write":
		return starlark.NewBuiltin("io.writer.write", w.writeCmd), nil
	case "close":
		return starlark.NewBuiltin("io.writer.close", w.closeCmd), nil
	}
	return nil, nil
}

func (w *starlarkWriter) AttrNames() []string {
	return []string{"write", "close"}
}

func (w *starlarkWriter) writeCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data starlark.Value
	if err := starlark.UnpackArgs("write", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, fmt.Errorf("io.writer.write: writer is closed")
	}

	switch v := data.(type) {
	case starlark.String:
		n, err := w.writer.Write([]byte(v))
		if err != nil {
			return nil, fmt.Errorf("io.writer.write: %w", err)
		}
		return starlark.MakeInt(n), nil
	case starlark.Bytes:
		n, err := w.writer.Write([]byte(v))
		if err != nil {
			return nil, fmt.Errorf("io.writer.write: %w", err)
		}
		return starlark.MakeInt(n), nil
	default:
		if sr, ok := v.(libkite.StarlarkReader); ok {
			src := sr.Reader()
			n, err := io.Copy(w.writer, src)

			// Automatically close the reader on completion
			if rc, ok := src.(io.Closer); ok {
				rc.Close()
			} else if closer, ok := sr.(io.Closer); ok {
				closer.Close()
			}

			if err != nil {
				return nil, fmt.Errorf("io.writer.write: pipe failed: %w", err)
			}
			return starlark.MakeInt64(n), nil
		}
		return nil, fmt.Errorf("io.writer.write: unsupported type %s (must be string, bytes, or io.reader)", data.Type())
	}
}

func (w *starlarkWriter) closeCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("close", args, kwargs); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("io.writer.close: %w", err)
	}
	return starlark.None, nil
}

package lsp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// DefaultMaxHeaderLineBytes bounds a single LSP JSON-RPC header line.
	DefaultMaxHeaderLineBytes = 16 << 10
	// DefaultMaxHeaders bounds the number of headers in one LSP frame.
	DefaultMaxHeaders = 64
	// DefaultMaxContentLength bounds one LSP JSON-RPC payload (16 MiB).
	DefaultMaxContentLength = 16 << 20
)

// FramingLimits configures byte and header limits enforced by NewBoundedReader.
// Zero fields fall back to package defaults.
type FramingLimits struct {
	MaxHeaderLineBytes int
	MaxHeaders         int
	MaxContentLength   int
}

func (l FramingLimits) withDefaults() FramingLimits {
	if l.MaxHeaderLineBytes <= 0 {
		l.MaxHeaderLineBytes = DefaultMaxHeaderLineBytes
	}
	if l.MaxHeaders <= 0 {
		l.MaxHeaders = DefaultMaxHeaders
	}
	if l.MaxContentLength <= 0 {
		l.MaxContentLength = DefaultMaxContentLength
	}
	return l
}

// NewBoundedReader wraps r to validate LSP Content-Length framing limits before
// forwarding raw frame bytes downstream.
func NewBoundedReader(r io.Reader, limits FramingLimits) io.Reader {
	limits = limits.withDefaults()
	return &boundedReader{
		br:     bufio.NewReaderSize(r, limits.MaxHeaderLineBytes+1),
		limits: limits,
	}
}

type boundedReader struct {
	br            *bufio.Reader
	limits        FramingLimits
	pending       []byte
	bodyRemaining int64
}

func (b *boundedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.pending) > 0 {
		n := copy(p, b.pending)
		b.pending = b.pending[n:]
		return n, nil
	}
	if b.bodyRemaining > 0 {
		maxRead := len(p)
		if int64(maxRead) > b.bodyRemaining {
			maxRead = int(b.bodyRemaining)
		}
		n, err := b.br.Read(p[:maxRead])
		b.bodyRemaining -= int64(n)
		if err == io.EOF && b.bodyRemaining > 0 {
			return n, io.ErrUnexpectedEOF
		}
		return n, err
	}

	if err := b.readNextHeaders(); err != nil {
		return 0, err
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

func (b *boundedReader) readNextHeaders() error {
	var raw bytes.Buffer
	headerCount := 0
	contentLength := int64(-1)

	for {
		line, err := b.br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			return fmt.Errorf("lsp frame: header line exceeds %d bytes", b.limits.MaxHeaderLineBytes)
		}
		if len(line) > b.limits.MaxHeaderLineBytes {
			return fmt.Errorf("lsp frame: header line exceeds %d bytes", b.limits.MaxHeaderLineBytes)
		}
		if err != nil {
			if err == io.EOF && len(line) == 0 && headerCount == 0 {
				return io.EOF
			}
			if err == io.EOF {
				return io.ErrUnexpectedEOF
			}
			return err
		}

		raw.Write(line)
		trimmed := strings.TrimRight(string(line), "\r\n")
		if trimmed == "" {
			if headerCount == 0 {
				return fmt.Errorf("lsp frame: missing headers")
			}
			if contentLength < 0 {
				return fmt.Errorf("lsp frame: missing Content-Length header")
			}
			b.pending = raw.Bytes()
			b.bodyRemaining = contentLength
			return nil
		}

		headerCount++
		if headerCount > b.limits.MaxHeaders {
			return fmt.Errorf("lsp frame: header count exceeds %d", b.limits.MaxHeaders)
		}

		name, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			return fmt.Errorf("lsp frame: malformed header %q", trimmed)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			length, parseErr := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if parseErr != nil || length < 0 {
				return fmt.Errorf("lsp frame: invalid Content-Length %q", strings.TrimSpace(val))
			}
			if length > int64(b.limits.MaxContentLength) {
				return fmt.Errorf("lsp frame: Content-Length %d exceeds maximum %d bytes", length, b.limits.MaxContentLength)
			}
			if contentLength >= 0 && contentLength != length {
				return fmt.Errorf("lsp frame: conflicting Content-Length headers (%d vs %d)", contentLength, length)
			}
			contentLength = length
		}
	}
}

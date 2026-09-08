package containers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// StreamType represents the multiplexed stream type byte in Docker's framing protocol.
type StreamType byte

const (
	StreamStdin  StreamType = 0
	StreamStdout StreamType = 1
	StreamStderr StreamType = 2
)

// DemuxStream reads Docker/Podman multiplexed 8-byte binary frames from r
// and routes payload bytes to stdout and/or stderr based on the stream type.
//
// Frame format:
//
//	[0]: Stream type (0: stdin, 1: stdout, 2: stderr)
//	[1..3]: Reserved (zeros)
//	[4..7]: uint32 big-endian payload length
//	[8..8+size]: Payload
//
// If the stream does not begin with an 8-byte frame header (e.g. raw non-multiplexed
// output), the initial bytes and remaining stream are forwarded directly to stdout.
func DemuxStream(r io.Reader, stdout, stderr io.Writer) error {
	header := make([]byte, 8)
	firstFrame := true

	for {
		_, err := io.ReadFull(r, header)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, io.ErrUnexpectedEOF) && firstFrame {
				// Partial raw text shorter than 8 bytes
				if stdout != nil {
					_, _ = stdout.Write(header)
				}
				return nil
			}
			return err
		}

		// Detect if this is unmultiplexed raw text
		if firstFrame {
			firstFrame = false
			if header[0] > 2 || header[1] != 0 || header[2] != 0 || header[3] != 0 {
				// Stream is raw non-multiplexed text
				if stdout != nil {
					if _, err := stdout.Write(header); err != nil {
						return err
					}
					if _, err := io.Copy(stdout, r); err != nil && !errors.Is(err, io.EOF) {
						return err
					}
				}
				return nil
			}
		}

		streamType := StreamType(header[0])
		size := binary.BigEndian.Uint32(header[4:8])

		var dest io.Writer
		switch streamType {
		case StreamStdout, StreamStdin:
			dest = stdout
		case StreamStderr:
			dest = stderr
		default:
			dest = stdout
		}

		if size > 0 {
			if dest != nil {
				if _, err := io.CopyN(dest, r, int64(size)); err != nil {
					return fmt.Errorf("containers: error reading frame payload: %w", err)
				}
			} else {
				if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
					return fmt.Errorf("containers: error discarding frame payload: %w", err)
				}
			}
		}
	}
}

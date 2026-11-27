package ssh

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/crypto/ssh"

	"github.com/project-starkite/starkite/libkite"
)

// upload uploads a file to remote hosts via SCP.
func (c *SSHClient) upload(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Src  string `name:"src" position:"0" required:"true"`
		Dst  string `name:"dst" position:"1" required:"true"`
		Mode string `name:"mode"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Mode == "" {
		p.Mode = "0644"
	}

	// Permission check for SSH upload - check jump host, each target host, and local source path
	if c.jumpHost != "" {
		if err := libkite.Check(thread, "ssh", "connect", "upload", c.jumpHost); err != nil {
			return nil, err
		}
	}
	for _, host := range c.hosts {
		if err := libkite.Check(thread, "ssh", "transfer", "upload", host+":"+p.Src+"->"+p.Dst); err != nil {
			return nil, err
		}
	}
	if err := libkite.Check(thread, "fs", "read", "upload", libkite.CanonicalizePath(p.Src)); err != nil {
		return nil, err
	}

	if c.dryRun {
		return c.dryRunTransferResults("upload", p.Src, p.Dst), nil
	}

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	if c.execPolicy == "concurrent" {
		return c.transferConcurrentContext(libkite.ThreadContext(thread), thread, "upload", p.Src, p.Dst, p.Mode)
	}
	return c.transferLinearContext(libkite.ThreadContext(thread), thread, "upload", p.Src, p.Dst, p.Mode)
}

// download downloads a file from remote hosts via SCP.
func (c *SSHClient) download(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Src string `name:"src" position:"0" required:"true"`
		Dst string `name:"dst" position:"1" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	// Permission check for SSH download - check jump host, each target host, and local destination path
	if c.jumpHost != "" {
		if err := libkite.Check(thread, "ssh", "connect", "download", c.jumpHost); err != nil {
			return nil, err
		}
	}
	for _, host := range c.hosts {
		if err := libkite.Check(thread, "ssh", "transfer", "download", host+":"+p.Src+"->"+p.Dst); err != nil {
			return nil, err
		}
	}
	if err := libkite.Check(thread, "fs", "write", "download", libkite.CanonicalizePath(p.Dst)); err != nil {
		return nil, err
	}

	if c.dryRun {
		return c.dryRunTransferResults("download", p.Src, p.Dst), nil
	}

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	if c.execPolicy == "concurrent" {
		return c.transferConcurrentContext(libkite.ThreadContext(thread), thread, "download", p.Src, p.Dst, "")
	}
	return c.transferLinearContext(libkite.ThreadContext(thread), thread, "download", p.Src, p.Dst, "")
}

func (c *SSHClient) dryRunTransferResults(op, src, dst string) starlark.Value {
	results := make([]starlark.Value, len(c.hosts))
	for i, host := range c.hosts {
		results[i] = starlarkstruct.FromStringDict(starlark.String("SSHTransferResult"), starlark.StringDict{
			"host":    starlark.String(host),
			"ok":      starlark.True,
			"bytes":   starlark.MakeInt(0),
			"src":     starlark.String(src),
			"dst":     starlark.String(dst),
			"dry_run": starlark.True,
		})
	}
	return starlark.NewList(results)
}

func (c *SSHClient) transferConcurrentContext(ctx context.Context, thread *starlark.Thread, op, src, dst, mode string) (starlark.Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]starlark.Value, len(c.hosts))
	errors := make([]error, len(c.hosts))
	var wg sync.WaitGroup

	var sem chan struct{}
	if c.execMaxWorkers > 0 {
		sem = make(chan struct{}, c.execMaxWorkers)
	}

	for i, host := range c.hosts {
		if sem != nil {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Wait()
				return nil, ctx.Err()
			}
		} else if err := ctx.Err(); err != nil {
			wg.Wait()
			return nil, err
		}
		wg.Add(1)
		go func(idx int, h string) {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}
			result, err := c.transferOnHostContext(ctx, thread, op, h, src, dst, mode)
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = result
		}(i, host)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("host %s: %w", c.hosts[i], err)
		}
	}

	return starlark.NewList(results), nil
}

func (c *SSHClient) transferLinearContext(ctx context.Context, thread *starlark.Thread, op, src, dst, mode string) (starlark.Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]starlark.Value, 0, len(c.hosts))

	for _, host := range c.hosts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := c.transferOnHostContext(ctx, thread, op, host, src, dst, mode)
		if err != nil {
			return nil, fmt.Errorf("host %s: %w", host, err)
		}
		results = append(results, result)
	}

	return starlark.NewList(results), nil
}

func (c *SSHClient) transferOnHostContext(ctx context.Context, thread *starlark.Thread, op, host, src, dst, mode string) (starlark.Value, error) {
	var n int64
	var err error

	switch op {
	case "upload":
		n, err = c.scpUploadToHostContext(ctx, thread, host, src, dst, mode)
	case "download":
		// For multi-host downloads, append host suffix to avoid collisions
		localDst := dst
		if len(c.hosts) > 1 {
			localDst = fmt.Sprintf("%s.%s", dst, host)
		}
		n, err = c.scpDownloadFromHostContext(ctx, thread, host, src, localDst)
		dst = localDst
	}

	if err != nil {
		return nil, err
	}

	return starlarkstruct.FromStringDict(starlark.String("SSHTransferResult"), starlark.StringDict{
		"host":  starlark.String(host),
		"ok":    starlark.True,
		"bytes": starlark.MakeInt(int(n)),
		"src":   starlark.String(src),
		"dst":   starlark.String(dst),
	}), nil
}

// scpShellQuote safely single-quotes a path for remote SCP invocation.
func scpShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (c *SSHClient) scpUploadToHost(host, localPath, remotePath, mode string) (int64, error) {
	return c.scpUploadToHostContext(libkite.ThreadContext(c.thread), c.thread, host, localPath, remotePath, mode)
}

// scpUploadToHostContext uploads a local file to a remote host using the SCP sink protocol.
func (c *SSHClient) scpUploadToHostContext(ctx context.Context, thread *starlark.Thread, host, localPath, remotePath, mode string) (int64, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read local file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to stat local file: %w", err)
	}
	size := info.Size()

	client, err := c.dialHostWithRetryContext(ctx, thread, host, "")
	if err != nil {
		return 0, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return 0, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	w, err := session.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	r, err := session.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stopWatch := watchSessionContext(ctx, session, client)
	defer stopWatch()

	if err := session.Start(fmt.Sprintf("scp -t -- %s", scpShellQuote(remotePath))); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("failed to start scp: %w", err)
	}

	// Read initial ACK
	if err := scpReadACK(r); err != nil {
		return 0, fmt.Errorf("scp initial ack: %w", err)
	}

	// Send file header: C<mode> <size> <filename>
	filename := filepath.Base(localPath)
	header := fmt.Sprintf("C%s %d %s\n", mode, size, filename)
	if _, err := w.Write([]byte(header)); err != nil {
		return 0, fmt.Errorf("scp write header: %w", err)
	}

	if err := scpReadACK(r); err != nil {
		return 0, fmt.Errorf("scp header ack: %w", err)
	}

	// Send file content
	n, err := io.Copy(w, f)
	if err != nil {
		return 0, fmt.Errorf("scp write content: %w", err)
	}

	// Send transfer complete (null byte)
	if _, err := w.Write([]byte{0}); err != nil {
		return 0, fmt.Errorf("scp write complete: %w", err)
	}

	if err := scpReadACK(r); err != nil {
		return 0, fmt.Errorf("scp final ack: %w", err)
	}

	w.Close()

	if err := session.Wait(); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return 0, fmt.Errorf("scp error (code %d)", exitErr.ExitStatus())
		}
		return 0, fmt.Errorf("scp session error: %w", err)
	}

	return n, nil
}

func (c *SSHClient) scpDownloadFromHost(host, remotePath, localPath string) (int64, error) {
	return c.scpDownloadFromHostContext(libkite.ThreadContext(c.thread), c.thread, host, remotePath, localPath)
}

// scpDownloadFromHostContext downloads a remote file to a local path using the SCP source protocol.
func (c *SSHClient) scpDownloadFromHostContext(ctx context.Context, thread *starlark.Thread, host, remotePath, localPath string) (int64, error) {
	client, err := c.dialHostWithRetryContext(ctx, thread, host, "")
	if err != nil {
		return 0, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return 0, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	w, err := session.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	r, err := session.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stopWatch := watchSessionContext(ctx, session, client)
	defer stopWatch()

	if err := session.Start(fmt.Sprintf("scp -f -- %s", scpShellQuote(remotePath))); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("failed to start scp: %w", err)
	}

	// Send initial ready signal
	if _, err := w.Write([]byte{0}); err != nil {
		return 0, fmt.Errorf("scp write ready: %w", err)
	}

	// Read file header: C<mode> <size> <filename>
	var mode string
	var size int64
	var filename string
	headerBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, headerBuf); err != nil {
		return 0, fmt.Errorf("scp read header type: %w", err)
	}

	switch headerBuf[0] {
	case 'C':
		// Read rest of header line
		line, err := readLine(r)
		if err != nil {
			return 0, fmt.Errorf("scp read header: %w", err)
		}
		if _, err := fmt.Sscanf(line, "%s %d %s", &mode, &size, &filename); err != nil {
			return 0, fmt.Errorf("scp parse header %q: %w", line, err)
		}
		if size < 0 {
			return 0, fmt.Errorf("scp invalid negative file size: %d", size)
		}
	case 0x01:
		line, _ := readLine(r)
		return 0, fmt.Errorf("scp error: %s", line)
	case 0x02:
		line, _ := readLine(r)
		return 0, fmt.Errorf("scp fatal error: %s", line)
	default:
		return 0, fmt.Errorf("scp unexpected response: 0x%02x", headerBuf[0])
	}

	// ACK the header
	if _, err := w.Write([]byte{0}); err != nil {
		return 0, fmt.Errorf("scp ack header: %w", err)
	}

	// Ensure parent directory exists
	if dir := filepath.Dir(localPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return 0, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Read file content
	outFile, err := os.Create(localPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create local file: %w", err)
	}
	outClosed := false
	defer func() {
		if !outClosed {
			_ = outFile.Close()
		}
	}()

	n, err := io.CopyN(outFile, r, size)
	if err != nil {
		return 0, fmt.Errorf("scp read content: %w", err)
	}

	// Read trailing null byte
	if _, err := io.ReadFull(r, headerBuf); err != nil {
		return 0, fmt.Errorf("scp read trailing null: %w", err)
	}

	// ACK the content
	if _, err := w.Write([]byte{0}); err != nil {
		return 0, fmt.Errorf("scp ack content: %w", err)
	}

	w.Close()

	if err := session.Wait(); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return 0, fmt.Errorf("scp error (code %d)", exitErr.ExitStatus())
		}
		return 0, fmt.Errorf("scp session error: %w", err)
	}

	outClosed = true
	if err := outFile.Close(); err != nil {
		return 0, fmt.Errorf("failed to close local file: %w", err)
	}

	return n, nil
}

// scpReadACK reads an SCP acknowledgement byte.
// 0x00 = success, 0x01 = warning, 0x02 = fatal error.
func scpReadACK(r io.Reader) error {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("failed to read ack: %w", err)
	}

	switch buf[0] {
	case 0:
		return nil
	case 1, 2:
		line, _ := readLine(r)
		return fmt.Errorf("scp error (code %d): %s", buf[0], line)
	default:
		return fmt.Errorf("unexpected ack byte: 0x%02x", buf[0])
	}
}

const maxSCPLineBytes = 4096

// readLine reads bytes until a newline, returning the line without the newline.
func readLine(r io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := r.Read(buf)
		if err != nil {
			return string(line), err
		}
		if buf[0] == '\n' {
			return string(line), nil
		}
		if len(line) >= maxSCPLineBytes {
			return "", fmt.Errorf("scp protocol line exceeds maximum length (%d bytes)", maxSCPLineBytes)
		}
		line = append(line, buf[0])
	}
}

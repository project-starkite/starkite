package ssh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/crypto/ssh"

	"github.com/project-starkite/starkite/starbase"
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

	// Permission check for SSH upload - check each host
	for _, host := range c.hosts {
		if err := starbase.Check(thread, "ssh", "upload", host+":"+p.Src+"->"+p.Dst); err != nil {
			return nil, err
		}
	}

	if c.dryRun {
		return c.dryRunTransferResults("upload", p.Src, p.Dst), nil
	}

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	if c.execPolicy == "concurrent" {
		return c.transferConcurrent("upload", p.Src, p.Dst, p.Mode)
	}
	return c.transferLinear("upload", p.Src, p.Dst, p.Mode)
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

	// Permission check for SSH download - check each host
	for _, host := range c.hosts {
		if err := starbase.Check(thread, "ssh", "download", host+":"+p.Src+"->"+p.Dst); err != nil {
			return nil, err
		}
	}

	if c.dryRun {
		return c.dryRunTransferResults("download", p.Src, p.Dst), nil
	}

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	if c.execPolicy == "concurrent" {
		return c.transferConcurrent("download", p.Src, p.Dst, "")
	}
	return c.transferLinear("download", p.Src, p.Dst, "")
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

func (c *SSHClient) transferConcurrent(op, src, dst, mode string) (starlark.Value, error) {
	results := make([]starlark.Value, len(c.hosts))
	errors := make([]error, len(c.hosts))
	var wg sync.WaitGroup

	for i, host := range c.hosts {
		wg.Add(1)
		go func(idx int, h string) {
			defer wg.Done()
			result, err := c.transferOnHost(op, h, src, dst, mode)
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

func (c *SSHClient) transferLinear(op, src, dst, mode string) (starlark.Value, error) {
	results := make([]starlark.Value, 0, len(c.hosts))

	for _, host := range c.hosts {
		result, err := c.transferOnHost(op, host, src, dst, mode)
		if err != nil {
			return nil, fmt.Errorf("host %s: %w", host, err)
		}
		results = append(results, result)
	}

	return starlark.NewList(results), nil
}

func (c *SSHClient) transferOnHost(op, host, src, dst, mode string) (starlark.Value, error) {
	var n int64
	var err error

	switch op {
	case "upload":
		n, err = c.scpUploadToHost(host, src, dst, mode)
	case "download":
		// For multi-host downloads, append host suffix to avoid collisions
		localDst := dst
		if len(c.hosts) > 1 {
			localDst = fmt.Sprintf("%s.%s", dst, host)
		}
		n, err = c.scpDownloadFromHost(host, src, localDst)
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

// scpUploadToHost uploads a local file to a remote host using the SCP sink protocol.
func (c *SSHClient) scpUploadToHost(host, localPath, remotePath, mode string) (int64, error) {
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

	client, err := c.dialHostWithRetry(host)
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

	if err := session.Start(fmt.Sprintf("scp -t %s", remotePath)); err != nil {
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
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return 0, fmt.Errorf("scp error (code %d)", exitErr.ExitStatus())
		}
		return 0, fmt.Errorf("scp session error: %w", err)
	}

	return n, nil
}

// scpDownloadFromHost downloads a remote file to a local path using the SCP source protocol.
func (c *SSHClient) scpDownloadFromHost(host, remotePath, localPath string) (int64, error) {
	client, err := c.dialHostWithRetry(host)
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

	if err := session.Start(fmt.Sprintf("scp -f %s", remotePath)); err != nil {
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
	defer outFile.Close()

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
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return 0, fmt.Errorf("scp error (code %d)", exitErr.ExitStatus())
		}
		return 0, fmt.Errorf("scp session error: %w", err)
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
		line = append(line, buf[0])
	}
}

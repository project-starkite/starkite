package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	gossh "golang.org/x/crypto/ssh"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ExecHandler is called for non-SCP exec requests.
// Returns stdout, stderr, and exit code.
type ExecHandler func(cmd string) (stdout, stderr string, exitCode int)

// SCPFile represents a file transferred via SCP.
type SCPFile struct {
	Name    string
	Mode    string
	Content []byte
}

// TestServer is an in-process SSH server for integration testing.
type TestServer struct {
	listener       net.Listener
	config         *gossh.ServerConfig
	hostSigner     gossh.Signer
	execHandler    ExecHandler
	files          map[string]*SCPFile // for SCP download (source mode)
	uploads        map[string]*SCPFile // populated by SCP upload (sink mode)
	passwords      map[string]string   // user → password
	authorizedKeys []gossh.PublicKey
	mu             sync.Mutex
	wg             sync.WaitGroup
	closed         chan struct{}
}

// NewTestServer creates a test SSH server listening on 127.0.0.1:0.
func NewTestServer() (*TestServer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	_ = pub

	hostSigner, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("create host signer: %w", err)
	}

	ts := &TestServer{
		hostSigner: hostSigner,
		files:      make(map[string]*SCPFile),
		uploads:    make(map[string]*SCPFile),
		passwords:  make(map[string]string),
		closed:     make(chan struct{}),
		execHandler: func(cmd string) (string, string, int) {
			return "", "", 0
		},
	}

	ts.config = &gossh.ServerConfig{
		PasswordCallback: func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			ts.mu.Lock()
			expected, ok := ts.passwords[conn.User()]
			ts.mu.Unlock()
			if ok && expected == string(password) {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
		PublicKeyCallback: func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			ts.mu.Lock()
			keys := ts.authorizedKeys
			ts.mu.Unlock()
			keyBytes := key.Marshal()
			for _, ak := range keys {
				if bytes.Equal(keyBytes, ak.Marshal()) {
					return nil, nil
				}
			}
			// If no authorized keys configured, accept any key
			if len(keys) == 0 {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
	}
	ts.config.AddHostKey(hostSigner)

	return ts, nil
}

// Start begins accepting connections.
func (ts *TestServer) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	ts.listener = ln
	go ts.acceptLoop()
	return nil
}

// Addr returns the server's listen address "127.0.0.1:<port>".
func (ts *TestServer) Addr() string {
	if ts.listener == nil {
		return ""
	}
	return ts.listener.Addr().String()
}

// Port returns the server's listen port.
func (ts *TestServer) Port() int {
	if ts.listener == nil {
		return 0
	}
	return ts.listener.Addr().(*net.TCPAddr).Port
}

// Close stops the server and waits for connections to drain.
func (ts *TestServer) Close() {
	select {
	case <-ts.closed:
		return // already closed
	default:
	}
	close(ts.closed)
	if ts.listener != nil {
		ts.listener.Close()
	}
	ts.wg.Wait()
}

// HandleExec sets the exec handler.
func (ts *TestServer) HandleExec(h ExecHandler) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.execHandler = h
}

// AddFile adds a file for SCP download.
func (ts *TestServer) AddFile(path string, content []byte, mode string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.files[path] = &SCPFile{
		Name:    filepath.Base(path),
		Mode:    mode,
		Content: content,
	}
}

// Uploaded returns a file that was uploaded via SCP, or nil.
func (ts *TestServer) Uploaded(path string) *SCPFile {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.uploads[path]
}

// AddPassword adds a user/password pair for authentication.
func (ts *TestServer) AddPassword(user, password string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.passwords[user] = password
}

// AddAuthorizedKey adds a public key for authentication.
func (ts *TestServer) AddAuthorizedKey(pubKey gossh.PublicKey) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.authorizedKeys = append(ts.authorizedKeys, pubKey)
}

func (ts *TestServer) acceptLoop() {
	for {
		conn, err := ts.listener.Accept()
		if err != nil {
			select {
			case <-ts.closed:
				return
			default:
				continue
			}
		}
		ts.wg.Add(1)
		go func() {
			defer ts.wg.Done()
			ts.handleConn(conn)
		}()
	}
}

func (ts *TestServer) handleConn(nConn net.Conn) {
	defer nConn.Close()

	sshConn, chans, reqs, err := gossh.NewServerConn(nConn, ts.config)
	if err != nil {
		return
	}
	defer sshConn.Close()

	// Handle global requests (keepalive etc)
	go func() {
		for req := range reqs {
			if req.Type == "keepalive@openssh.com" {
				req.Reply(true, nil)
			} else {
				req.Reply(false, nil)
			}
		}
	}()

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			newCh.Reject(gossh.UnknownChannelType, "unknown channel type")
			continue
		}
		ch, sessionReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		go ts.handleSession(ch, sessionReqs)
	}
}

func (ts *TestServer) handleSession(ch gossh.Channel, reqs <-chan *gossh.Request) {
	defer ch.Close()

	for req := range reqs {
		switch req.Type {
		case "exec":
			if req.WantReply {
				req.Reply(true, nil)
			}
			ts.handleExec(ch, req.Payload)
			return
		case "env":
			if req.WantReply {
				req.Reply(true, nil)
			}
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func (ts *TestServer) handleExec(ch gossh.Channel, payload []byte) {
	// Parse command from wire format: uint32 length + command bytes
	if len(payload) < 4 {
		ts.sendExitStatus(ch, 1)
		return
	}
	cmdLen := binary.BigEndian.Uint32(payload[:4])
	if len(payload) < int(4+cmdLen) {
		ts.sendExitStatus(ch, 1)
		return
	}
	cmd := string(payload[4 : 4+cmdLen])

	// Route SCP commands
	if path, ok := strings.CutPrefix(cmd, "scp -t "); ok {
		ts.handleSCPSink(ch, path)
		return
	}
	if path, ok := strings.CutPrefix(cmd, "scp -f "); ok {
		ts.handleSCPSource(ch, path)
		return
	}

	// Regular exec
	ts.mu.Lock()
	handler := ts.execHandler
	ts.mu.Unlock()

	stdout, stderr, code := handler(cmd)
	if stdout != "" {
		io.WriteString(ch, stdout)
	}
	if stderr != "" {
		io.WriteString(ch.Stderr(), stderr)
	}
	ts.sendExitStatus(ch, code)
}

func (ts *TestServer) sendExitStatus(ch gossh.Channel, code int) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(code))
	ch.SendRequest("exit-status", false, buf[:])
}

// handleSCPSink receives an upload from the client.
func (ts *TestServer) handleSCPSink(ch gossh.Channel, destPath string) {
	// Send initial ACK (ready)
	ch.Write([]byte{0})

	// Read header: C<mode> <size> <filename>\n
	header, err := readLineByte(ch)
	if err != nil {
		ts.sendExitStatus(ch, 1)
		return
	}

	// Parse header
	var mode string
	var size int64
	var filename string
	if _, err := fmt.Sscanf(header, "C%s %d %s", &mode, &size, &filename); err != nil {
		ts.sendExitStatus(ch, 1)
		return
	}

	// ACK the header
	ch.Write([]byte{0})

	// Read file content
	content := make([]byte, size)
	if _, err := io.ReadFull(ch, content); err != nil {
		ts.sendExitStatus(ch, 1)
		return
	}

	// Read trailing null byte
	trail := make([]byte, 1)
	io.ReadFull(ch, trail)

	// ACK the content
	ch.Write([]byte{0})

	// Store upload
	ts.mu.Lock()
	ts.uploads[destPath] = &SCPFile{
		Name:    filename,
		Mode:    mode,
		Content: content,
	}
	ts.mu.Unlock()

	ts.sendExitStatus(ch, 0)
}

// handleSCPSource sends a file to the client.
func (ts *TestServer) handleSCPSource(ch gossh.Channel, srcPath string) {
	// Read ready signal from client
	ready := make([]byte, 1)
	if _, err := io.ReadFull(ch, ready); err != nil {
		ts.sendExitStatus(ch, 1)
		return
	}

	// Look up file
	ts.mu.Lock()
	f, ok := ts.files[srcPath]
	ts.mu.Unlock()

	if !ok {
		// Send error
		ch.Write([]byte{0x01})
		io.WriteString(ch, "file not found\n")
		ts.sendExitStatus(ch, 1)
		return
	}

	// Send header: C<mode> <size> <filename>\n
	header := fmt.Sprintf("C%s %d %s\n", f.Mode, len(f.Content), f.Name)
	ch.Write([]byte(header))

	// Read ACK from client
	ack := make([]byte, 1)
	io.ReadFull(ch, ack)

	// Send content
	ch.Write(f.Content)

	// Send trailing null
	ch.Write([]byte{0})

	// Read final ACK from client
	io.ReadFull(ch, ack)

	ts.sendExitStatus(ch, 0)
}

// readLineByte reads bytes until newline, returns the line without the newline.
func readLineByte(r io.Reader) (string, error) {
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

// GenerateClientKey creates an ed25519 key pair, writes the private key to a temp file,
// and returns the file path and public key.
func GenerateClientKey(dir string) (privateKeyPath string, pubKey gossh.PublicKey, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("generate key: %w", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", nil, fmt.Errorf("marshal private key: %w", err)
	}

	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	keyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, pemBlock, 0600); err != nil {
		return "", nil, fmt.Errorf("write key file: %w", err)
	}

	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return "", nil, fmt.Errorf("convert public key: %w", err)
	}

	return keyPath, sshPub, nil
}

// --- Starlark wrapper ---

// StarlarkTestServer wraps TestServer for Starlark access.
type StarlarkTestServer struct {
	server    *TestServer
	thread    *starlark.Thread
	keyPaths  []string // track generated keys for this server
	tempDirs  []string // track temp dirs
}

var (
	_ starlark.Value    = (*StarlarkTestServer)(nil)
	_ starlark.HasAttrs = (*StarlarkTestServer)(nil)
)

func (s *StarlarkTestServer) String() string        { return "<ssh.test_server>" }
func (s *StarlarkTestServer) Type() string          { return "ssh.test_server" }
func (s *StarlarkTestServer) Freeze()               {}
func (s *StarlarkTestServer) Truth() starlark.Bool  { return starlark.True }
func (s *StarlarkTestServer) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: ssh.test_server") }

func (s *StarlarkTestServer) Attr(name string) (starlark.Value, error) {
	switch name {
	case "start":
		return starlark.NewBuiltin("ssh.test_server.start", s.startMethod), nil
	case "shutdown":
		return starlark.NewBuiltin("ssh.test_server.shutdown", s.shutdownMethod), nil
	case "port":
		return starlark.NewBuiltin("ssh.test_server.port", s.portMethod), nil
	case "addr":
		return starlark.NewBuiltin("ssh.test_server.addr", s.addrMethod), nil
	case "add_file":
		return starlark.NewBuiltin("ssh.test_server.add_file", s.addFileMethod), nil
	case "uploaded":
		return starlark.NewBuiltin("ssh.test_server.uploaded", s.uploadedMethod), nil
	case "handle_exec":
		return starlark.NewBuiltin("ssh.test_server.handle_exec", s.handleExecMethod), nil
	}
	return nil, nil
}

func (s *StarlarkTestServer) AttrNames() []string {
	return []string{"add_file", "addr", "handle_exec", "port", "shutdown", "start", "uploaded"}
}

func (s *StarlarkTestServer) startMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := s.server.Start(); err != nil {
		return nil, fmt.Errorf("ssh.test_server.start: %w", err)
	}
	return starlark.None, nil
}

func (s *StarlarkTestServer) shutdownMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	s.server.Close()
	// Clean up temp dirs
	for _, d := range s.tempDirs {
		os.RemoveAll(d)
	}
	return starlark.None, nil
}

func (s *StarlarkTestServer) portMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.MakeInt(s.server.Port()), nil
}

func (s *StarlarkTestServer) addrMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(s.server.Addr()), nil
}

func (s *StarlarkTestServer) addFileMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content, mode string
	mode = "0644"

	if len(args) >= 1 {
		if p, ok := starlark.AsString(args[0]); ok {
			path = p
		}
	}
	if len(args) >= 2 {
		if c, ok := starlark.AsString(args[1]); ok {
			content = c
		}
	}
	if len(args) >= 3 {
		if m, ok := starlark.AsString(args[2]); ok {
			mode = m
		}
	}
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "path":
			if p, ok := starlark.AsString(kv[1]); ok {
				path = p
			}
		case "content":
			if c, ok := starlark.AsString(kv[1]); ok {
				content = c
			}
		case "mode":
			if m, ok := starlark.AsString(kv[1]); ok {
				mode = m
			}
		}
	}

	if path == "" {
		return nil, fmt.Errorf("ssh.test_server.add_file: path is required")
	}

	s.server.AddFile(path, []byte(content), mode)
	return starlark.None, nil
}

func (s *StarlarkTestServer) uploadedMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ssh.test_server.uploaded: expected 1 argument (path)")
	}
	path, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("ssh.test_server.uploaded: path must be a string")
	}

	f := s.server.Uploaded(path)
	if f == nil {
		return starlark.None, nil
	}

	return starlarkstruct.FromStringDict(starlark.String("SCPFile"), starlark.StringDict{
		"name":    starlark.String(f.Name),
		"mode":    starlark.String(f.Mode),
		"content": starlark.String(string(f.Content)),
	}), nil
}

func (s *StarlarkTestServer) handleExecMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ssh.test_server.handle_exec: expected 1 argument (callable)")
	}
	callable, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("ssh.test_server.handle_exec: argument must be callable")
	}

	s.server.HandleExec(func(cmd string) (string, string, int) {
		result, err := starlark.Call(thread, callable, starlark.Tuple{starlark.String(cmd)}, nil)
		if err != nil {
			return "", fmt.Sprintf("handler error: %v", err), 1
		}

		// Accept tuple (stdout, stderr, exit_code)
		if tup, ok := result.(starlark.Tuple); ok && tup.Len() == 3 {
			stdout, _ := starlark.AsString(tup.Index(0))
			stderr, _ := starlark.AsString(tup.Index(1))
			code, _ := starlark.AsInt32(tup.Index(2))
			return stdout, stderr, code
		}

		return "", "handler must return (stdout, stderr, exit_code) tuple", 1
	})

	return starlark.None, nil
}

// testserverFactory creates a StarlarkTestServer.
// Usage: ssh.test_server(user="testuser", password="pass")
func (m *Module) testserverFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var user, password string
	user = "testuser"
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "user":
			if u, ok := starlark.AsString(kv[1]); ok {
				user = u
			}
		case "password":
			if p, ok := starlark.AsString(kv[1]); ok {
				password = p
			}
		}
	}

	ts, err := NewTestServer()
	if err != nil {
		return nil, fmt.Errorf("ssh.test_server: %w", err)
	}

	if password != "" {
		ts.AddPassword(user, password)
	}

	return &StarlarkTestServer{
		server: ts,
		thread: thread,
	}, nil
}

// testKeyFactory generates an ed25519 key pair for testing.
// Usage: key = ssh.test_key()  → key.path
func (m *Module) testKeyFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	dir, err := os.MkdirTemp("", "starkite-ssh-testkey-*")
	if err != nil {
		return nil, fmt.Errorf("ssh.test_key: %w", err)
	}

	keyPath, _, err := GenerateClientKey(dir)
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("ssh.test_key: %w", err)
	}

	return starlarkstruct.FromStringDict(starlark.String("SSHTestKey"), starlark.StringDict{
		"path": starlark.String(keyPath),
		"port": starlark.MakeInt(0), // placeholder for compat
	}), nil
}

// testserverFactoryForTest creates a TestServer for Go tests with t.Cleanup.
func newTestServerForTest(t interface{ TempDir() string; Cleanup(func()); Helper() }) *TestServer {
	t.Helper()
	ts, err := NewTestServer()
	if err != nil {
		panic(fmt.Sprintf("newTestServer: %v", err))
	}
	if err := ts.Start(); err != nil {
		panic(fmt.Sprintf("testserver.Start: %v", err))
	}
	t.Cleanup(ts.Close)
	return ts
}

// clientKeyForTest generates an ed25519 key pair for Go tests.
func clientKeyForTest(t interface{ TempDir() string; Helper() }) (privateKeyPath string, pubKey gossh.PublicKey) {
	t.Helper()
	path, pub, err := GenerateClientKey(t.TempDir())
	if err != nil {
		panic(fmt.Sprintf("clientKey: %v", err))
	}
	return path, pub
}

// portFromAddr extracts the port number from a "host:port" address.
func portFromAddr(addr string) int {
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	return port
}

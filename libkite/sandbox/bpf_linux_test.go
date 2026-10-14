//go:build linux

package sandbox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBuildNetworkFilter_InstructionCount(t *testing.T) {
	filter, err := BuildNetworkFilter()
	if err != nil {
		t.Fatalf("BuildNetworkFilter() returned unexpected error: %v", err)
	}

	// 4 header + 15 denied (3 process memory + 12 SysV IPC) + 3 SYS_KILL + 6 SYS_SOCKET + 2 terminal = 30
	const expectedLen = 30
	if len(filter) != expectedLen {
		t.Errorf("len(filter) = %d, want %d", len(filter), expectedLen)
	}
}

func TestBuildSeccompFilter_OptionsAndStructure(t *testing.T) {
	// 1. Always-On Host Network Filter (BlockNetwork=false, NoExec=false)
	// 4 header + 15 denied + 3 SYS_KILL + 2 terminal = 24
	hostFilter, err := BuildSeccompFilter(SeccompFilterOptions{BlockNetwork: false})
	if err != nil {
		t.Fatalf("BuildSeccompFilter(BlockNetwork=false) error: %v", err)
	}
	if len(hostFilter) != 24 {
		t.Errorf("len(hostFilter) = %d, want 24", len(hostFilter))
	}

	// 2. NoExec + BlockNetwork Filter (BlockNetwork=true, NoExec=true)
	// 4 header + 17 denied + 3 SYS_KILL + 6 SYS_SOCKET + 2 terminal = 32
	noExecFilter, err := BuildSeccompFilter(SeccompFilterOptions{BlockNetwork: true, NoExec: true})
	if err != nil {
		t.Fatalf("BuildSeccompFilter(NoExec=true) error: %v", err)
	}
	if len(noExecFilter) != 32 {
		t.Errorf("len(noExecFilter) = %d, want 32", len(noExecFilter))
	}

	// 3. AuditLog mode verifies terminal deny instruction uses SECCOMP_RET_LOG
	auditFilter, err := BuildSeccompFilter(SeccompFilterOptions{BlockNetwork: true, AuditLog: true})
	if err != nil {
		t.Fatalf("BuildSeccompFilter(AuditLog=true) error: %v", err)
	}
	lastInsn := auditFilter[len(auditFilter)-1]
	if lastInsn.Code != unix.BPF_RET|unix.BPF_K || lastInsn.K != seccompRetLog {
		t.Errorf("expected terminal deny instruction with SECCOMP_RET_LOG (%#x), got %+v", seccompRetLog, lastInsn)
	}

	// 4. Verify header and terminal instructions on standard network filter
	filter, err := BuildNetworkFilter()
	if err != nil {
		t.Fatalf("BuildNetworkFilter() error: %v", err)
	}

	// [0] Load architecture: seccomp_data.arch (offset 4)
	if filter[0].Code != unix.BPF_LD|unix.BPF_W|unix.BPF_ABS || filter[0].K != 4 {
		t.Errorf("filter[0] invalid load arch: %+v", filter[0])
	}

	// [1] Check arch == auditArch
	if filter[1].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[1].K != auditArch {
		t.Errorf("filter[1] expected jump auditArch (%#x), got %#x", auditArch, filter[1].K)
	}
	if filter[1].Jt != 1 || filter[1].Jf != 0 {
		t.Errorf("filter[1] expected Jt=1 Jf=0, got Jt=%d Jf=%d", filter[1].Jt, filter[1].Jf)
	}

	// [2] Architecture mismatch returns EINVAL
	if filter[2].Code != unix.BPF_RET|unix.BPF_K || filter[2].K != unix.SECCOMP_RET_ERRNO|uint32(unix.EINVAL) {
		t.Errorf("filter[2] expected RET ERRNO EINVAL, got %+v", filter[2])
	}

	// [3] Load syscall number: seccomp_data.nr (offset 0)
	if filter[3].Code != unix.BPF_LD|unix.BPF_W|unix.BPF_ABS || filter[3].K != 0 {
		t.Errorf("filter[3] invalid load nr: %+v", filter[3])
	}

	idxAllow := len(filter) - 2
	idxDeny := len(filter) - 1

	if filter[idxAllow].Code != unix.BPF_RET|unix.BPF_K || filter[idxAllow].K != unix.SECCOMP_RET_ALLOW {
		t.Errorf("filter[idxAllow] expected RET ALLOW, got %+v", filter[idxAllow])
	}
	if filter[idxDeny].Code != unix.BPF_RET|unix.BPF_K || filter[idxDeny].K != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
		t.Errorf("filter[idxDeny] expected RET ERRNO EPERM, got %+v", filter[idxDeny])
	}
}

func TestBuildSeccompFilter_VerificationAcyclic(t *testing.T) {
	optionCombos := []SeccompFilterOptions{
		{BlockNetwork: false, NoExec: false, AuditLog: false},
		{BlockNetwork: true, NoExec: false, AuditLog: false},
		{BlockNetwork: false, NoExec: true, AuditLog: false},
		{BlockNetwork: true, NoExec: true, AuditLog: false},
		{BlockNetwork: true, NoExec: true, AuditLog: true},
	}

	for _, opts := range optionCombos {
		filter, err := BuildSeccompFilter(opts)
		if err != nil {
			t.Fatalf("BuildSeccompFilter(%+v) error: %v", opts, err)
		}

		n := len(filter)
		for i, insn := range filter {
			isJmp := (insn.Code & 0x07) == unix.BPF_JMP
			if isJmp {
				targetTrue := i + 1 + int(insn.Jt)
				targetFalse := i + 1 + int(insn.Jf)

				if targetTrue >= n {
					t.Errorf("opts=%+v: instruction %d has out-of-bounds Jt target %d (len=%d)", opts, i, targetTrue, n)
				}
				if targetFalse >= n {
					t.Errorf("opts=%+v: instruction %d has out-of-bounds Jf target %d (len=%d)", opts, i, targetFalse, n)
				}
			}
		}
	}
}

func TestApplySeccompFilter_Empty(t *testing.T) {
	if err := ApplySeccompFilter(nil); err != nil {
		t.Errorf("ApplySeccompFilter(nil) returned error: %v", err)
	}
	if err := ApplySeccompFilter([]unix.SockFilter{}); err != nil {
		t.Errorf("ApplySeccompFilter([]) returned error: %v", err)
	}
}

// TestSeccompBPF_Integration spawns dedicated sub-processes to attach filters
// and test socket, IPC, process memory, signal, and NoExec enforcement.
func TestSeccompBPF_Integration(t *testing.T) {
	switch os.Getenv("STARKITE_TEST_SECCOMP_CHILD") {
	case "network":
		runSeccompChildTest(t)
		return
	case "host_always_on":
		runSeccompHostAlwaysOnChildTest(t)
		return
	case "no_exec":
		runSeccompNoExecChildTest(t)
		return
	}

	for _, mode := range []string{"network", "host_always_on", "no_exec"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestSeccompBPF_Integration")
			cmd.Env = append(os.Environ(), "STARKITE_TEST_SECCOMP_CHILD="+mode)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Seccomp child test (%s) failed: %v\nOutput:\n%s", mode, err, string(out))
			}
			t.Logf("Seccomp integration child (%s) passed:\n%s", mode, string(out))
		})
	}
}

func runSeccompChildTest(t *testing.T) {
	// Baseline check: TCP works before attaching filter
	conn, err := net.DialTimeout("tcp", "1.1.1.1:80", 2*time.Second)
	if err == nil {
		conn.Close()
	}

	// 1. Build and apply filter
	filter, err := BuildNetworkFilter()
	if err != nil {
		t.Fatalf("BuildNetworkFilter failed: %v", err)
	}

	if err := ApplySeccompFilter(filter); err != nil {
		t.Fatalf("ApplySeccompFilter failed: %v", err)
	}

	// 2. Test in-process IPv4 TCP: must be blocked with EPERM
	_, err = net.DialTimeout("tcp", "1.1.1.1:80", 200*time.Millisecond)
	if err == nil {
		t.Fatalf("net.Dial TCP unexpectedly succeeded through Seccomp filter")
	}
	if !errors.Is(err, syscall.EPERM) && !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected EPERM for TCP socket creation, got %v", err)
	}

	// 3. Test in-process UDP: must be blocked with EPERM
	_, err = net.DialTimeout("udp", "8.8.8.8:53", 200*time.Millisecond)
	if err == nil {
		t.Fatalf("net.Dial UDP unexpectedly succeeded through Seccomp filter")
	}
	if !errors.Is(err, syscall.EPERM) && !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected EPERM for UDP socket creation, got %v", err)
	}

	// 4. Test AF_UNIX domain socket: must be PERMITTED
	tmpSock := filepath.Join(os.TempDir(), fmt.Sprintf("starkite_seccomp_%d.sock", os.Getpid()))
	defer os.Remove(tmpSock)
	l, err := net.Listen("unix", tmpSock)
	if err != nil {
		t.Fatalf("AF_UNIX domain socket was blocked: %v", err)
	}
	l.Close()

	// 5. Test cross-process memory protection: SYS_PROCESS_VM_READV must be blocked with EPERM
	var localBuf [8]byte
	localIov := []unix.Iovec{{Base: &localBuf[0], Len: 8}}
	remoteIov := []unix.RemoteIovec{{Base: 0x1000, Len: 8}}
	_, err = unix.ProcessVMReadv(1, localIov, remoteIov, 0)
	if err == nil {
		t.Errorf("expected ProcessVMReadv to fail with EPERM, but succeeded")
	} else if !errors.Is(err, syscall.EPERM) && !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected EPERM for ProcessVMReadv, got %v", err)
	}

	// 6. Test signal scoping: SYS_KILL targeting external pid > 0 must be blocked with EPERM
	err = unix.Kill(1, 0) // Signal probe to init (PID 1)
	if err == nil {
		t.Errorf("expected Kill(1, 0) to fail with EPERM, but succeeded")
	} else if !errors.Is(err, syscall.EPERM) && !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected EPERM for Kill(1, 0), got %v", err)
	}

	// 7. Test signal scoping: SYS_KILL targeting pid == 0 (own process group) must be PERMITTED
	err = unix.Kill(0, 0) // Signal probe to own process group
	if err != nil && (errors.Is(err, syscall.EPERM) || errors.Is(err, os.ErrPermission)) {
		t.Errorf("expected Kill(0, 0) to be permitted, but got %v", err)
	}

	// 8. Test System V IPC blocking: SYS_SHMGET, SYS_SEMGET, SYS_MSGGET must be blocked with EPERM
	for _, sysNr := range []uintptr{unix.SYS_SHMGET, unix.SYS_SEMGET, unix.SYS_MSGGET} {
		_, _, errno := unix.Syscall(sysNr, 0, 4096, unix.IPC_CREAT|0o600)
		if errno != unix.EPERM {
			t.Errorf("expected System V IPC syscall %d to fail with EPERM, got %v", sysNr, errno)
		}
	}

	// 9. Test normal filesystem operations: must be unimpeded
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("starkite_io_%d.txt", os.Getpid()))
	defer os.Remove(tmpFile)
	if err := os.WriteFile(tmpFile, []byte("seccomp-ok"), 0o600); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}
	readBytes, err := os.ReadFile(tmpFile)
	if err != nil || string(readBytes) != "seccomp-ok" {
		t.Fatalf("os.ReadFile failed: %v", err)
	}

	// 10. Test child process inheritance across execve
	echoCmd := exec.Command("/bin/echo", "seccomp-child-ok")
	echoOut, err := echoCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execve child failed: %v", err)
	}
	if string(echoOut) != "seccomp-child-ok\n" {
		t.Errorf("unexpected child output: %q", string(echoOut))
	}
}

func runSeccompHostAlwaysOnChildTest(t *testing.T) {
	// Build filter with BlockNetwork=false (simulating NetworkHost)
	filter, err := BuildSeccompFilter(SeccompFilterOptions{BlockNetwork: false})
	if err != nil {
		t.Fatalf("BuildSeccompFilter(BlockNetwork=false) failed: %v", err)
	}
	if err := ApplySeccompFilter(filter); err != nil {
		t.Fatalf("ApplySeccompFilter failed: %v", err)
	}

	// 1. AF_INET socket creation MUST be permitted under NetworkHost
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("expected AF_INET socket to be permitted when BlockNetwork=false, got: %v", err)
	}
	unix.Close(fd)

	// 2. Process memory tampering MUST still be blocked with EPERM
	var localBuf [8]byte
	localIov := []unix.Iovec{{Base: &localBuf[0], Len: 8}}
	remoteIov := []unix.RemoteIovec{{Base: 0x1000, Len: 8}}
	if _, err := unix.ProcessVMReadv(1, localIov, remoteIov, 0); !errors.Is(err, syscall.EPERM) {
		t.Errorf("expected ProcessVMReadv to fail with EPERM under NetworkHost, got %v", err)
	}

	// 3. External signal targeting MUST still be blocked with EPERM
	if err := unix.Kill(1, 0); !errors.Is(err, syscall.EPERM) {
		t.Errorf("expected Kill(1, 0) to fail with EPERM under NetworkHost, got %v", err)
	}

	// 4. System V IPC MUST still be blocked with EPERM
	if _, _, errno := unix.Syscall(unix.SYS_SHMGET, 0, 4096, unix.IPC_CREAT|0o600); errno != unix.EPERM {
		t.Errorf("expected SYS_SHMGET to fail with EPERM under NetworkHost, got %v", errno)
	}
}

func runSeccompNoExecChildTest(t *testing.T) {
	filter, err := BuildSeccompFilter(SeccompFilterOptions{BlockNetwork: true, NoExec: true})
	if err != nil {
		t.Fatalf("BuildSeccompFilter(NoExec=true) failed: %v", err)
	}
	if err := ApplySeccompFilter(filter); err != nil {
		t.Fatalf("ApplySeccompFilter failed: %v", err)
	}

	// Attempting to spawn any child process via execve must fail with EPERM
	cmd := exec.Command("/bin/echo", "should-not-run")
	err = cmd.Run()
	if err == nil {
		t.Fatalf("expected exec.Command to fail under NoExec=true, but it succeeded")
	}
	if !errors.Is(err, syscall.EPERM) && !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected EPERM/ErrPermission under NoExec=true, got: %v", err)
	}
}

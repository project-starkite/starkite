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

	const expectedLen = 15
	if len(filter) != expectedLen {
		t.Errorf("len(filter) = %d, want %d", len(filter), expectedLen)
	}
}

func TestBuildNetworkFilter_InstructionStructure(t *testing.T) {
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

	// [4] Check SYS_PTRACE
	if filter[4].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[4].K != uint32(unix.SYS_PTRACE) {
		t.Errorf("filter[4] expected jump SYS_PTRACE (%d), got %d", unix.SYS_PTRACE, filter[4].K)
	}

	// [5] Return EPERM for ptrace
	if filter[5].Code != unix.BPF_RET|unix.BPF_K || filter[5].K != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
		t.Errorf("filter[5] expected RET ERRNO EPERM, got %+v", filter[5])
	}

	// [6] Check SYS_SOCKET
	if filter[6].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[6].K != uint32(unix.SYS_SOCKET) {
		t.Errorf("filter[6] expected jump SYS_SOCKET (%d), got %d", unix.SYS_SOCKET, filter[6].K)
	}
	if filter[6].Jt != 0 || filter[6].Jf != 7 {
		t.Errorf("filter[6] expected Jt=0 Jf=7, got Jt=%d Jf=%d", filter[6].Jt, filter[6].Jf)
	}

	// [7] Load args[0] (domain)
	if filter[7].Code != unix.BPF_LD|unix.BPF_W|unix.BPF_ABS || filter[7].K != 16 {
		t.Errorf("filter[7] expected load args[0] at offset 16, got %+v", filter[7])
	}

	// [8] AF_INET
	if filter[8].K != unix.AF_INET || filter[8].Jt != 4 {
		t.Errorf("filter[8] expected AF_INET jump 4, got %+v", filter[8])
	}
	// [9] AF_INET6
	if filter[9].K != unix.AF_INET6 || filter[9].Jt != 3 {
		t.Errorf("filter[9] expected AF_INET6 jump 3, got %+v", filter[9])
	}
	// [10] AF_PACKET
	if filter[10].K != unix.AF_PACKET || filter[10].Jt != 2 {
		t.Errorf("filter[10] expected AF_PACKET jump 2, got %+v", filter[10])
	}
	// [11] AF_NETLINK
	if filter[11].K != unix.AF_NETLINK || filter[11].Jt != 1 {
		t.Errorf("filter[11] expected AF_NETLINK jump 1, got %+v", filter[11])
	}

	// [12] AF_UNIX allowed -> RET ALLOW
	if filter[12].Code != unix.BPF_RET|unix.BPF_K || filter[12].K != unix.SECCOMP_RET_ALLOW {
		t.Errorf("filter[12] expected RET ALLOW, got %+v", filter[12])
	}

	// [13] Blocked network socket -> RET ERRNO EPERM
	if filter[13].Code != unix.BPF_RET|unix.BPF_K || filter[13].K != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
		t.Errorf("filter[13] expected RET ERRNO EPERM, got %+v", filter[13])
	}

	// [14] Default non-socket syscalls -> RET ALLOW
	if filter[14].Code != unix.BPF_RET|unix.BPF_K || filter[14].K != unix.SECCOMP_RET_ALLOW {
		t.Errorf("filter[14] expected RET ALLOW, got %+v", filter[14])
	}
}

func TestBuildNetworkFilter_VerificationAcyclic(t *testing.T) {
	filter, err := BuildNetworkFilter()
	if err != nil {
		t.Fatalf("BuildNetworkFilter() error: %v", err)
	}

	n := len(filter)
	for i, insn := range filter {
		// Verify that all jump targets are strictly forward and within bounds
		isJmp := (insn.Code & 0x07) == unix.BPF_JMP
		if isJmp {
			targetTrue := i + 1 + int(insn.Jt)
			targetFalse := i + 1 + int(insn.Jf)

			if targetTrue >= n {
				t.Errorf("instruction %d has out-of-bounds Jt target %d (len=%d)", i, targetTrue, n)
			}
			if targetFalse >= n {
				t.Errorf("instruction %d has out-of-bounds Jf target %d (len=%d)", i, targetFalse, n)
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

// TestSeccompBPF_Integration spawns a dedicated sub-process to attach the filter
// and test socket blocking so the main test runner process is not restricted.
func TestSeccompBPF_Integration(t *testing.T) {
	if os.Getenv("STARKITE_TEST_SECCOMP_CHILD") == "1" {
		runSeccompChildTest(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSeccompBPF_Integration")
	cmd.Env = append(os.Environ(), "STARKITE_TEST_SECCOMP_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Seccomp child test failed: %v\nOutput:\n%s", err, string(out))
	}
	t.Logf("Seccomp integration child process passed:\n%s", string(out))
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

	// 5. Test normal filesystem operations: must be unimpeded
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("starkite_io_%d.txt", os.Getpid()))
	defer os.Remove(tmpFile)
	if err := os.WriteFile(tmpFile, []byte("seccomp-ok"), 0o600); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}
	readBytes, err := os.ReadFile(tmpFile)
	if err != nil || string(readBytes) != "seccomp-ok" {
		t.Fatalf("os.ReadFile failed: %v", err)
	}

	// 6. Test child process inheritance across execve
	echoCmd := exec.Command("/bin/echo", "seccomp-child-ok")
	echoOut, err := echoCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execve child failed: %v", err)
	}
	if string(echoOut) != "seccomp-child-ok\n" {
		t.Errorf("unexpected child output: %q", string(echoOut))
	}
}

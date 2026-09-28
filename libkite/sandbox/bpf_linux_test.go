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

	const expectedLen = 18
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

	// [4] Check SYS_PTRACE -> jump 11 to [16] (RET_EPERM)
	if filter[4].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[4].K != uint32(unix.SYS_PTRACE) {
		t.Errorf("filter[4] expected jump SYS_PTRACE (%d), got %d", unix.SYS_PTRACE, filter[4].K)
	}
	if filter[4].Jt != 11 || filter[4].Jf != 0 {
		t.Errorf("filter[4] expected Jt=11 Jf=0, got Jt=%d Jf=%d", filter[4].Jt, filter[4].Jf)
	}

	// [5] Check SYS_PROCESS_VM_READV -> jump 10 to [16] (RET_EPERM)
	if filter[5].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[5].K != uint32(unix.SYS_PROCESS_VM_READV) {
		t.Errorf("filter[5] expected jump SYS_PROCESS_VM_READV (%d), got %d", unix.SYS_PROCESS_VM_READV, filter[5].K)
	}
	if filter[5].Jt != 10 || filter[5].Jf != 0 {
		t.Errorf("filter[5] expected Jt=10 Jf=0, got Jt=%d Jf=%d", filter[5].Jt, filter[5].Jf)
	}

	// [6] Check SYS_PROCESS_VM_WRITEV -> jump 9 to [16] (RET_EPERM)
	if filter[6].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[6].K != uint32(unix.SYS_PROCESS_VM_WRITEV) {
		t.Errorf("filter[6] expected jump SYS_PROCESS_VM_WRITEV (%d), got %d", unix.SYS_PROCESS_VM_WRITEV, filter[6].K)
	}
	if filter[6].Jt != 9 || filter[6].Jf != 0 {
		t.Errorf("filter[6] expected Jt=9 Jf=0, got Jt=%d Jf=%d", filter[6].Jt, filter[6].Jf)
	}

	// [7] Check SYS_KILL -> if equal continue to [8], else jump 2 to [10] (SYS_SOCKET)
	if filter[7].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[7].K != uint32(unix.SYS_KILL) {
		t.Errorf("filter[7] expected jump SYS_KILL (%d), got %d", unix.SYS_KILL, filter[7].K)
	}
	if filter[7].Jt != 0 || filter[7].Jf != 2 {
		t.Errorf("filter[7] expected Jt=0 Jf=2, got Jt=%d Jf=%d", filter[7].Jt, filter[7].Jf)
	}

	// [8] Load args[0] (lower 32-bits of pid)
	if filter[8].Code != unix.BPF_LD|unix.BPF_W|unix.BPF_ABS || filter[8].K != 16 {
		t.Errorf("filter[8] expected load args[0] at offset 16, got %+v", filter[8])
	}

	// [9] Check pid == 0 -> jump 7 to [17] (ALLOW), else jump 6 to [16] (EPERM)
	if filter[9].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[9].K != 0 {
		t.Errorf("filter[9] expected JEQ 0, got %+v", filter[9])
	}
	if filter[9].Jt != 7 || filter[9].Jf != 6 {
		t.Errorf("filter[9] expected Jt=7 Jf=6, got Jt=%d Jf=%d", filter[9].Jt, filter[9].Jf)
	}

	// [10] Check SYS_SOCKET -> if equal continue to [11], else jump 6 to [17] (ALLOW)
	if filter[10].Code != unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K || filter[10].K != uint32(unix.SYS_SOCKET) {
		t.Errorf("filter[10] expected jump SYS_SOCKET (%d), got %d", unix.SYS_SOCKET, filter[10].K)
	}
	if filter[10].Jt != 0 || filter[10].Jf != 6 {
		t.Errorf("filter[10] expected Jt=0 Jf=6, got Jt=%d Jf=%d", filter[10].Jt, filter[10].Jf)
	}

	// [11] Load args[0] (domain)
	if filter[11].Code != unix.BPF_LD|unix.BPF_W|unix.BPF_ABS || filter[11].K != 16 {
		t.Errorf("filter[11] expected load args[0] at offset 16, got %+v", filter[11])
	}

	// [12] AF_INET -> jump 3 to [16] (EPERM)
	if filter[12].K != unix.AF_INET || filter[12].Jt != 3 {
		t.Errorf("filter[12] expected AF_INET jump 3, got %+v", filter[12])
	}
	// [13] AF_INET6 -> jump 2 to [16] (EPERM)
	if filter[13].K != unix.AF_INET6 || filter[13].Jt != 2 {
		t.Errorf("filter[13] expected AF_INET6 jump 2, got %+v", filter[13])
	}
	// [14] AF_PACKET -> jump 1 to [16] (EPERM)
	if filter[14].K != unix.AF_PACKET || filter[14].Jt != 1 {
		t.Errorf("filter[14] expected AF_PACKET jump 1, got %+v", filter[14])
	}
	// [15] AF_NETLINK -> jump 0 to [16] (EPERM), else jump 1 to [17] (ALLOW)
	if filter[15].K != unix.AF_NETLINK || filter[15].Jt != 0 || filter[15].Jf != 1 {
		t.Errorf("filter[15] expected AF_NETLINK Jt=0 Jf=1, got %+v", filter[15])
	}

	// [16] Blocked operation -> RET ERRNO EPERM
	if filter[16].Code != unix.BPF_RET|unix.BPF_K || filter[16].K != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
		t.Errorf("filter[16] expected RET ERRNO EPERM, got %+v", filter[16])
	}

	// [17] Allowed operation / Default -> RET ALLOW
	if filter[17].Code != unix.BPF_RET|unix.BPF_K || filter[17].K != unix.SECCOMP_RET_ALLOW {
		t.Errorf("filter[17] expected RET ALLOW, got %+v", filter[17])
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

	// 8. Test normal filesystem operations: must be unimpeded
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("starkite_io_%d.txt", os.Getpid()))
	defer os.Remove(tmpFile)
	if err := os.WriteFile(tmpFile, []byte("seccomp-ok"), 0o600); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}
	readBytes, err := os.ReadFile(tmpFile)
	if err != nil || string(readBytes) != "seccomp-ok" {
		t.Fatalf("os.ReadFile failed: %v", err)
	}

	// 9. Test child process inheritance across execve
	echoCmd := exec.Command("/bin/echo", "seccomp-child-ok")
	echoOut, err := echoCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execve child failed: %v", err)
	}
	if string(echoOut) != "seccomp-child-ok\n" {
		t.Errorf("unexpected child output: %q", string(echoOut))
	}
}

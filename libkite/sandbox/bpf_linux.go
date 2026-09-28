//go:build linux

package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

func bpfStmt(code uint16, k uint32) unix.SockFilter {
	return unix.SockFilter{Code: code, K: k}
}

func bpfJump(code uint16, k uint32, jt, jf uint8) unix.SockFilter {
	return unix.SockFilter{Code: code, Jt: jt, Jf: jf, K: k}
}

// BuildNetworkFilter returns an 18-instruction classic BPF (cBPF) filter
// that enforces process boundary hardening and network isolation:
//  1. Validates architecture against auditArch (rejecting mismatch with EINVAL).
//  2. Intercepts SYS_PTRACE, SYS_PROCESS_VM_READV, and SYS_PROCESS_VM_WRITEV
//     to prevent cross-process memory inspection or injection (returning EPERM).
//  3. Intercepts SYS_KILL, allowing signals only targeting the current process group
//     (pid == 0) and rejecting signals targeting external host PIDs (pid != 0) with EPERM.
//     SYS_TGKILL and SYS_TKILL are explicitly preserved for Go runtime goroutine
//     preemption (SIGURG) and glibc raise().
//  4. Intercepts SYS_SOCKET to block network socket creation (AF_INET, AF_INET6,
//     AF_PACKET, AF_NETLINK) with EPERM, while permitting local AF_UNIX domain sockets.
//  5. Allows all other non-targeted system calls.
//
// Architecture audit constants are resolved at compile time via architecture-specific
// build tags (bpf_linux_amd64.go, bpf_linux_arm64.go).
func BuildNetworkFilter() ([]unix.SockFilter, error) {
	if !isSupportedArch {
		return nil, errUnsupportedArch
	}

	filter := []unix.SockFilter{
		// [0] Load architecture: seccomp_data.arch (offset 4)
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 4),
		// [1] If arch == auditArch, continue to [3], else reject at [2]
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, auditArch, 1, 0),
		// [2] Architecture mismatch -> return EINVAL
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.EINVAL)),

		// [3] Load syscall number: seccomp_data.nr (offset 0)
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 0),

		// [4] Check SYS_PTRACE: if equal jump 11 to [16] (RET_EPERM), else continue to [5]
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(unix.SYS_PTRACE), 11, 0),
		// [5] Check SYS_PROCESS_VM_READV: if equal jump 10 to [16] (RET_EPERM), else continue to [6]
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(unix.SYS_PROCESS_VM_READV), 10, 0),
		// [6] Check SYS_PROCESS_VM_WRITEV: if equal jump 9 to [16] (RET_EPERM), else continue to [7]
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(unix.SYS_PROCESS_VM_WRITEV), 9, 0),

		// [7] Check SYS_KILL: if equal continue to [8], else jump 2 to [10] (SYS_SOCKET)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(unix.SYS_KILL), 0, 2),
		// [8] Load args[0] (lower 32-bits of pid, offset 16 on 64-bit Little-Endian)
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 16),
		// [9] pid == 0 (own process group) -> jump 7 to [17] (ALLOW), pid != 0 -> jump 6 to [16] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, 0, 7, 6),

		// [10] Check SYS_SOCKET: if equal continue to [11], else jump 6 to [17] (ALLOW)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(unix.SYS_SOCKET), 0, 6),
		// [11] Load args[0] (lower 32-bits of socket domain, offset 16 on 64-bit Little-Endian)
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 16),
		// [12] domain == AF_INET (IPv4) -> jump 3 to [16] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_INET, 3, 0),
		// [13] domain == AF_INET6 (IPv6) -> jump 2 to [16] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_INET6, 2, 0),
		// [14] domain == AF_PACKET -> jump 1 to [16] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_PACKET, 1, 0),
		// [15] domain == AF_NETLINK -> jump 0 to [16] (EPERM), else jump 1 to [17] (ALLOW, e.g. AF_UNIX)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_NETLINK, 0, 1),

		// [16] Blocked operation (memory tampering, external signal, or network socket) -> return EPERM
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM)),

		// [17] Default / Allowed operation -> return ALLOW
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW),
	}

	return filter, nil
}

// ApplySeccompFilter installs the BPF filter on the current process and all future
// threads using PR_SET_NO_NEW_PRIVS and seccomp(SECCOMP_SET_MODE_FILTER, SECCOMP_FILTER_FLAG_TSYNC).
// If the seccomp(2) syscall returns ENOSYS, it falls back to prctl(PR_SET_SECCOMP).
func ApplySeccompFilter(filter []unix.SockFilter) error {
	if len(filter) == 0 {
		return nil
	}

	prog := unix.SockFprog{
		Len:    uint16(len(filter)),
		Filter: &filter[0],
	}

	// 1. Enforce no new privileges so unprivileged processes can attach seccomp filters
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("sandbox: prctl(PR_SET_NO_NEW_PRIVS) failed: %w", err)
	}

	// 2. Attach filter with TSYNC (thread synchronization) across all Go runtime OS threads
	_, _, err := unix.Syscall(
		unix.SYS_SECCOMP,
		unix.SECCOMP_SET_MODE_FILTER,
		unix.SECCOMP_FILTER_FLAG_TSYNC,
		uintptr(unsafe.Pointer(&prog)),
	)
	if err == 0 {
		return nil
	}

	// If seccomp(2) syscall is not available (kernels < 3.17), fallback to prctl
	if err == unix.ENOSYS {
		if prctlErr := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&prog)), 0, 0); prctlErr != nil {
			return fmt.Errorf("sandbox: prctl(PR_SET_SECCOMP) fallback failed: %w", prctlErr)
		}
		return nil
	}

	return fmt.Errorf("sandbox: seccomp(SECCOMP_FILTER_FLAG_TSYNC) failed: %w", err)
}

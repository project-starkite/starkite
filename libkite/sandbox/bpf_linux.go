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

// BuildNetworkFilter returns a 15-instruction classic BPF (cBPF) filter
// that intercepts SYS_SOCKET to block network socket creation (AF_INET, AF_INET6,
// AF_PACKET, AF_NETLINK) while permitting local AF_UNIX domain sockets.
// It also intercepts SYS_PTRACE to prevent process memory inspection.
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

		// [4] Check SYS_PTRACE: if equal jump to [5], else continue to [6]
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(unix.SYS_PTRACE), 0, 1),
		// [5] Return EPERM for ptrace
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM)),

		// [6] Check SYS_SOCKET: if equal continue to [7], else jump 7 to [14] (ALLOW)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(unix.SYS_SOCKET), 0, 7),

		// [7] Load args[0] (lower 32-bits of socket domain, offset 16 on 64-bit Little-Endian)
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 16),

		// [8] domain == AF_INET (IPv4) -> jump 4 to [13] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_INET, 4, 0),
		// [9] domain == AF_INET6 (IPv6) -> jump 3 to [13] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_INET6, 3, 0),
		// [10] domain == AF_PACKET -> jump 2 to [13] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_PACKET, 2, 0),
		// [11] domain == AF_NETLINK -> jump 1 to [13] (EPERM)
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_NETLINK, 1, 0),

		// [12] Allowed domain (e.g. AF_UNIX) -> return ALLOW
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW),

		// [13] Blocked network socket -> return EPERM
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM)),

		// [14] Default: return ALLOW for non-socket syscalls
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

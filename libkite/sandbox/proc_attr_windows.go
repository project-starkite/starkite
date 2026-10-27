//go:build windows

package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// Win32 extended startup attribute keys.
	procThreadAttributeHandleList           = 0x00020002
	procThreadAttributeMitigationPolicy     = 0x00020007
	procThreadAttributeSecurityCapabilities = 0x00020009
	procThreadAttributeChildProcess         = 0x0002000E

	// Child process creation restriction policy flags.
	processCreationChildProcessRestricted = 0x01

	// Process mitigation policy flags (64-bit bitmask).
	processCreationMitigationPolicySEHOPEnableAlwaysOn        uint64 = 0x00000001 << 2
	processCreationMitigationPolicyStrictHandleChecksAlwaysOn uint64 = 0x00000001 << 8
)

// windowsStartupAttrs holds the StartupInfoEx and pinned memory buffers required
// across the CreateProcessW call.
type windowsStartupAttrs struct {
	startupInfoEx     windows.StartupInfoEx
	attrListContainer *windows.ProcThreadAttributeListContainer
	childPolicyVal    uint32
	mitigationVal     uint64
	inheritedHandles  []windows.Handle
}

// buildWindowsStartupAttrs constructs a StartupInfoEx configured with handle whitelisting,
// strict handle mitigation policies, optional AppContainer capabilities, and optional NoExec child process creation lockdown.
func buildWindowsStartupAttrs(
	spec *ExecutionSpec,
	stdin, stdout, stderr windows.Handle,
	secCaps *securityCapabilities,
) (*windowsStartupAttrs, error) {
	var maxAttrs uint32 = 2 // handle list + mitigation policy
	if spec.NoExec {
		maxAttrs++
	}
	if secCaps != nil {
		maxAttrs++
	}

	attrList, err := windows.NewProcThreadAttributeList(maxAttrs)
	if err != nil {
		return nil, fmt.Errorf("sandbox: NewProcThreadAttributeList failed: %w", err)
	}

	attrs := &windowsStartupAttrs{
		attrListContainer: attrList,
		childPolicyVal:    processCreationChildProcessRestricted,
		mitigationVal:     processCreationMitigationPolicyStrictHandleChecksAlwaysOn | processCreationMitigationPolicySEHOPEnableAlwaysOn,
	}

	// Collect unique valid stdio handles for explicit inheritance whitelisting.
	seen := make(map[windows.Handle]bool)
	for _, h := range []windows.Handle{stdin, stdout, stderr} {
		if h != 0 && h != windows.InvalidHandle && !seen[h] {
			seen[h] = true
			attrs.inheritedHandles = append(attrs.inheritedHandles, h)
		}
	}

	if len(attrs.inheritedHandles) > 0 {
		err = attrList.Update(
			procThreadAttributeHandleList,
			unsafe.Pointer(&attrs.inheritedHandles[0]),
			uintptr(len(attrs.inheritedHandles))*unsafe.Sizeof(attrs.inheritedHandles[0]),
		)
		if err != nil {
			attrList.Delete()
			return nil, fmt.Errorf("sandbox: UpdateProcThreadAttribute (HANDLE_LIST) failed: %w", err)
		}
	}

	err = attrList.Update(
		procThreadAttributeMitigationPolicy,
		unsafe.Pointer(&attrs.mitigationVal),
		unsafe.Sizeof(attrs.mitigationVal),
	)
	if err != nil {
		attrList.Delete()
		return nil, fmt.Errorf("sandbox: UpdateProcThreadAttribute (MITIGATION_POLICY) failed: %w", err)
	}

	if secCaps != nil {
		err = attrList.Update(
			procThreadAttributeSecurityCapabilities,
			unsafe.Pointer(secCaps),
			unsafe.Sizeof(*secCaps),
		)
		if err != nil {
			attrList.Delete()
			return nil, fmt.Errorf("sandbox: UpdateProcThreadAttribute (SECURITY_CAPABILITIES) failed: %w", err)
		}
	}

	if spec.NoExec {
		err = attrList.Update(
			procThreadAttributeChildProcess,
			unsafe.Pointer(&attrs.childPolicyVal),
			unsafe.Sizeof(attrs.childPolicyVal),
		)
		if err != nil {
			attrList.Delete()
			return nil, fmt.Errorf("sandbox: UpdateProcThreadAttribute (CHILD_PROCESS_POLICY) failed: %w", err)
		}
	}

	attrs.startupInfoEx.StartupInfo.Cb = uint32(unsafe.Sizeof(attrs.startupInfoEx))
	attrs.startupInfoEx.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	attrs.startupInfoEx.StartupInfo.StdInput = stdin
	attrs.startupInfoEx.StartupInfo.StdOutput = stdout
	attrs.startupInfoEx.StartupInfo.StdErr = stderr
	attrs.startupInfoEx.ProcThreadAttributeList = attrList.List()

	return attrs, nil
}

func (a *windowsStartupAttrs) close() {
	if a.attrListContainer != nil {
		a.attrListContainer.Delete()
		a.attrListContainer = nil
	}
}

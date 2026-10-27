//go:build windows

package sandbox

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	defaultWindowsMaxPIDs = 256

	// Job Object Basic UI Restrictions flags.
	jobObjectUILimitHandles          = 0x00000001
	jobObjectUILimitReadClipboard    = 0x00000002
	jobObjectUILimitWriteClipboard   = 0x00000004
	jobObjectUILimitSystemParameters = 0x00000008
	jobObjectUILimitDisplaySettings  = 0x00000010
	jobObjectUILimitGlobalAtoms      = 0x00000020
	jobObjectUILimitDesktop          = 0x00000040
	jobObjectUILimitExitWindows      = 0x00000080

	// Job Object CPU Rate Control flags.
	jobObjectCpuRateControlEnable  = 0x00000001
	jobObjectCpuRateControlHardCap = 0x00000004

	// Job Object Information Classes.
	jobObjectBasicUIRestrictionsClass         = 4
	jobObjectAssociateCompletionPortInfoClass = 7
	jobObjectCpuRateControlInformationClass   = 15

	// Job Object I/O Completion Port message IDs.
	jobObjectMsgEndOfJobTime        = 1
	jobObjectMsgEndOfProcessTime    = 2
	jobObjectMsgActiveProcessLimit  = 3
	jobObjectMsgActiveProcessZero   = 4
	jobObjectMsgNewProcess          = 6
	jobObjectMsgExitProcess         = 7
	jobObjectMsgAbnormalExitProcess = 8
	jobObjectMsgProcessMemoryLimit  = 9
	jobObjectMsgJobMemoryLimit      = 10
)

type jobObjectBasicUIRestrictions struct {
	UIRestrictionsClass uint32
}

type jobObjectCpuRateControlInformation struct {
	ControlFlags uint32
	CpuRate      uint32
}

type jobObjectAssociateCompletionPort struct {
	CompletionKey  uintptr
	CompletionPort windows.Handle
}

// windowsJob encapsulates a kernel Job Object and its associated I/O Completion Port.
type windowsJob struct {
	handle      windows.Handle
	ioPort      windows.Handle
	killedByOOM atomic.Bool
	doneCh      chan struct{}
}

// newWindowsJob creates and configures a Windows Job Object according to spec constraints.
func newWindowsJob(spec *ExecutionSpec) (*windowsJob, error) {
	jobHandle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("sandbox: CreateJobObject failed: %w", err)
	}

	j := &windowsJob{
		handle: jobHandle,
		doneCh: make(chan struct{}),
	}

	if err := j.configureLimits(spec); err != nil {
		_ = windows.CloseHandle(jobHandle)
		return nil, err
	}

	if err := j.configureUIRestrictions(); err != nil {
		_ = windows.CloseHandle(jobHandle)
		return nil, err
	}

	if spec.MaxCPUs > 0 {
		if err := j.configureCPURate(spec.MaxCPUs); err != nil {
			_ = windows.CloseHandle(jobHandle)
			return nil, err
		}
	}

	if err := j.attachCompletionPort(); err != nil {
		_ = windows.CloseHandle(jobHandle)
		return nil, err
	}

	return j, nil
}

func (j *windowsJob) configureLimits(spec *ExecutionSpec) error {
	var flags uint32 = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
		windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION |
		windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS

	activeLimit := uint32(defaultWindowsMaxPIDs)
	if spec.NoExec {
		activeLimit = 1
	} else if spec.MaxPIDs > 0 {
		activeLimit = uint32(spec.MaxPIDs)
	}

	extLimits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags:         flags,
			ActiveProcessLimit: activeLimit,
		},
	}

	if spec.MaxMemoryMB > 0 {
		memBytes := uintptr(spec.MaxMemoryMB) * 1024 * 1024
		extLimits.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
		extLimits.JobMemoryLimit = memBytes
		extLimits.ProcessMemoryLimit = memBytes
	}

	_, err := windows.SetInformationJobObject(
		j.handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&extLimits)),
		uint32(unsafe.Sizeof(extLimits)),
	)
	if err != nil {
		return fmt.Errorf("sandbox: SetInformationJobObject (ExtendedLimitInformation) failed: %w", err)
	}
	return nil
}

func (j *windowsJob) configureUIRestrictions() error {
	ui := jobObjectBasicUIRestrictions{
		UIRestrictionsClass: jobObjectUILimitHandles |
			jobObjectUILimitReadClipboard |
			jobObjectUILimitWriteClipboard |
			jobObjectUILimitSystemParameters |
			jobObjectUILimitDisplaySettings |
			jobObjectUILimitGlobalAtoms |
			jobObjectUILimitDesktop |
			jobObjectUILimitExitWindows,
	}
	_, err := windows.SetInformationJobObject(
		j.handle,
		jobObjectBasicUIRestrictionsClass,
		uintptr(unsafe.Pointer(&ui)),
		uint32(unsafe.Sizeof(ui)),
	)
	if err != nil {
		return fmt.Errorf("sandbox: SetInformationJobObject (BasicUIRestrictions) failed: %w", err)
	}
	return nil
}

func (j *windowsJob) configureCPURate(maxCPUs float64) error {
	numCPU := float64(runtime.NumCPU())
	if numCPU < 1 {
		numCPU = 1
	}
	// Windows CpuRate is expressed as cycles per 10,000 intervals (1% = 100, 100% = 10000).
	rate := uint32((maxCPUs / numCPU) * 10000)
	if rate < 1 {
		rate = 1
	}
	if rate > 10000 {
		rate = 10000
	}

	cpuInfo := jobObjectCpuRateControlInformation{
		ControlFlags: jobObjectCpuRateControlEnable | jobObjectCpuRateControlHardCap,
		CpuRate:      rate,
	}
	_, err := windows.SetInformationJobObject(
		j.handle,
		jobObjectCpuRateControlInformationClass,
		uintptr(unsafe.Pointer(&cpuInfo)),
		uint32(unsafe.Sizeof(cpuInfo)),
	)
	if err != nil {
		return fmt.Errorf("sandbox: SetInformationJobObject (CpuRateControl) failed: %w", err)
	}
	return nil
}

func (j *windowsJob) attachCompletionPort() error {
	ioPort, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		return fmt.Errorf("sandbox: CreateIoCompletionPort failed: %w", err)
	}
	j.ioPort = ioPort

	assoc := jobObjectAssociateCompletionPort{
		CompletionKey:  uintptr(j.handle),
		CompletionPort: ioPort,
	}
	_, err = windows.SetInformationJobObject(
		j.handle,
		jobObjectAssociateCompletionPortInfoClass,
		uintptr(unsafe.Pointer(&assoc)),
		uint32(unsafe.Sizeof(assoc)),
	)
	if err != nil {
		_ = windows.CloseHandle(ioPort)
		j.ioPort = 0
		return fmt.Errorf("sandbox: SetInformationJobObject (AssociateCompletionPort) failed: %w", err)
	}

	go j.monitorCompletionPort()
	return nil
}

func (j *windowsJob) monitorCompletionPort() {
	defer close(j.doneCh)
	for {
		var msgID uint32
		var completionKey uintptr
		var overlapped *windows.Overlapped

		err := windows.GetQueuedCompletionStatus(j.ioPort, &msgID, &completionKey, &overlapped, windows.INFINITE)
		if err != nil {
			return
		}
		if completionKey == 0 && msgID == 0 {
			return
		}

		switch msgID {
		case jobObjectMsgProcessMemoryLimit, jobObjectMsgJobMemoryLimit:
			j.killedByOOM.Store(true)
			_ = windows.TerminateJobObject(j.handle, 137)
		case jobObjectMsgActiveProcessZero:
			return
		}
	}
}

func (j *windowsJob) assignProcess(procHandle windows.Handle) error {
	if err := windows.AssignProcessToJobObject(j.handle, procHandle); err != nil {
		return fmt.Errorf("sandbox: AssignProcessToJobObject failed: %w", err)
	}
	return nil
}

func (j *windowsJob) terminate(exitCode uint32) {
	if j.handle != 0 {
		_ = windows.TerminateJobObject(j.handle, exitCode)
	}
}

func (j *windowsJob) close() {
	if j.ioPort != 0 {
		_ = windows.PostQueuedCompletionStatus(j.ioPort, 0, 0, nil)
		<-j.doneCh
		_ = windows.CloseHandle(j.ioPort)
		j.ioPort = 0
	}
	if j.handle != 0 {
		_ = windows.TerminateJobObject(j.handle, 1)
		_ = windows.CloseHandle(j.handle)
		j.handle = 0
	}
}

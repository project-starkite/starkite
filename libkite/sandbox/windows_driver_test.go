//go:build windows

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsDriver_RegistrationAndAvailability(t *testing.T) {
	d, err := Get(DriverWindows)
	if err != nil {
		t.Fatalf("Get(DriverWindows) failed: %v", err)
	}
	if d.Name() != DriverWindows {
		t.Errorf("d.Name() = %q, want %q", d.Name(), DriverWindows)
	}

	alias, err := Get(DriverAppContainer)
	if err != nil {
		t.Fatalf("Get(DriverAppContainer) failed: %v", err)
	}
	if alias.Name() != DriverWindows {
		t.Errorf("alias.Name() = %q, want %q", alias.Name(), DriverWindows)
	}

	if !d.Available() {
		t.Logf("WindowsDriver reports unavailable on this host: %v", checkWindowsOSBaseline())
	}
}

func TestWindowsDriver_BasicExec(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	spec := &ExecutionSpec{
		Command: []string{"cmd.exe", "/c", "echo", "hello-windows-sandbox"},
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (stderr: %q)", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "hello-windows-sandbox") {
		t.Errorf("Stdout = %q, want substring 'hello-windows-sandbox'", res.Stdout)
	}
}

func TestWindowsDriver_NoExecBlocksChildProcess(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	// cmd.exe is launched, but when it attempts to spawn whoami.exe,
	// PROC_THREAD_ATTRIBUTE_CHILD_PROCESS_POLICY (PROCESS_CREATION_CHILD_PROCESS_RESTRICTED)
	// causes the kernel to deny CreateProcess with Access is denied.
	spec := &ExecutionSpec{
		Command: []string{"cmd.exe", "/c", "whoami.exe"},
		NoExec:  true,
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	// Process may return non-zero exit code or error
	if err == nil && res.ExitCode == 0 {
		t.Errorf("expected child process creation to be blocked under NoExec=true, got exit 0 (stdout: %q)", res.Stdout)
	}
	combined := ""
	exitCode := -1
	if res != nil {
		combined = res.Stdout + res.Stderr
		exitCode = res.ExitCode
	}
	t.Logf("NoExec child block result: exit=%v output=%q err=%v", exitCode, combined, err)
}

func TestWindowsDriver_JobObjectTimeoutAndOrphanCleanup(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	spec := &ExecutionSpec{
		Command: []string{"cmd.exe", "/c", "for /L %i in (1,1,10000000) do @verify>nul"},
		Timeout: 500 * time.Millisecond,
	}

	res, err := d.Exec(context.Background(), spec)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if res == nil || !res.TimedOut {
		t.Errorf("expected res.TimedOut == true, got %v", res)
	}
}

func TestWindowsDriver_JobObjectMemoryLimitOOM(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	// Request 16MB limit while PowerShell allocates 64MB array
	spec := &ExecutionSpec{
		Command:     []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$a = New-Object byte[] (64*1024*1024)"},
		MaxMemoryMB: 16,
		Timeout:     10 * time.Second,
	}

	res, _ := d.Exec(context.Background(), spec)
	if res != nil {
		t.Logf("OOM test result: exit=%d killedByOOM=%v stderr=%q", res.ExitCode, res.KilledByOOM, res.Stderr)
		if !res.KilledByOOM && res.ExitCode == 0 {
			t.Errorf("expected memory limit termination, but process exited with code 0")
		}
	}
}

func TestWindowsDriver_OSBaselineCheck(t *testing.T) {
	// With override env var set, baseline check always passes
	os.Setenv("STARKITE_WINDOWS_ALLOW_LEGACY_BUILD", "1")
	defer os.Unsetenv("STARKITE_WINDOWS_ALLOW_LEGACY_BUILD")

	if err := checkWindowsOSBaseline(); err != nil {
		t.Errorf("checkWindowsOSBaseline() with override failed: %v", err)
	}
}

func TestWindowsDriver_AppContainerLifecycle(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	profile, err := newAppContainerProfile(NetworkNone)
	if err != nil {
		t.Fatalf("newAppContainerProfile(NetworkNone) failed: %v", err)
	}
	defer profile.close()

	if profile.name == "" || !strings.HasPrefix(profile.name, "starkite.sbx.") {
		t.Errorf("profile.name = %q, expected prefix 'starkite.sbx.'", profile.name)
	}
	if profile.appContainer == nil {
		t.Errorf("expected non-nil AppContainer SID")
	} else {
		t.Logf("profile.appContainer: %s", profile.appContainer.String())
	}
	if len(profile.capabilities) != 0 {
		t.Errorf("expected 0 capabilities for NetworkNone, got %d", len(profile.capabilities))
	}
}

func TestWindowsDriver_AppContainerNetworkCapabilities(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	profile, err := newAppContainerProfile(NetworkHost)
	if err != nil {
		t.Fatalf("newAppContainerProfile(NetworkHost) failed: %v", err)
	}
	defer profile.close()

	if len(profile.capabilities) != 2 {
		t.Errorf("expected 2 capabilities for NetworkHost, got %d", len(profile.capabilities))
	}

	secCaps := profile.securityCapabilitiesPtr()
	if secCaps == nil {
		t.Fatalf("securityCapabilitiesPtr returned nil")
	}
	if secCaps.CapabilityCount != 2 {
		t.Errorf("secCaps.CapabilityCount = %d, want 2", secCaps.CapabilityCount)
	}
}

func TestWindowsDriver_NetworkNone_BlocksSocket(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	// Under NetworkNone (AppContainer with zero capability SIDs),
	// WFP blocks all socket creation with WSAEACCES (10013).
	spec := &ExecutionSpec{
		Command: []string{
			"powershell.exe", "-NoProfile", "-NonInteractive",
			"-Command", "try { $c = New-Object System.Net.Sockets.TcpClient('1.1.1.1', 53); exit 0 } catch { exit 10013 }",
		},
		Network: NetworkNone,
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	if err == nil && res != nil && res.ExitCode == 0 {
		t.Errorf("expected socket connection to be blocked under NetworkNone, got exit 0")
	}
	var exitCode int
	if res != nil {
		exitCode = res.ExitCode
	}
	t.Logf("NetworkNone socket block result: exit=%d (err: %v)", exitCode, err)
}

func TestWindowsDriver_AclGrantAndRevocation(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	profile, err := newAppContainerProfile(NetworkNone)
	if err != nil {
		t.Fatalf("newAppContainerProfile failed: %v", err)
	}
	defer profile.close()

	tmpDir, err := os.MkdirTemp("", "starkite-acl-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Test granting read-only access to file
	if err := grantPathAccess(testFile, profile.appContainer, readOnlyAccessMask); err != nil {
		t.Errorf("grantPathAccess(file) failed: %v", err)
	}

	// Test granting read-write access to directory
	if err := grantPathAccess(tmpDir, profile.appContainer, readWriteAccessMask); err != nil {
		t.Errorf("grantPathAccess(dir) failed: %v", err)
	}

	// Test revoking access
	if err := revokePathAccess(testFile, profile.appContainer); err != nil {
		t.Errorf("revokePathAccess(file) failed: %v", err)
	}
	if err := revokePathAccess(tmpDir, profile.appContainer); err != nil {
		t.Errorf("revokePathAccess(dir) failed: %v", err)
	}
}

func TestWindowsDriver_W_XOR_X_DenyExecute(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	profile, err := newAppContainerProfile(NetworkNone)
	if err != nil {
		t.Fatalf("newAppContainerProfile failed: %v", err)
	}
	defer profile.close()

	tmpDir, err := os.MkdirTemp("", "starkite-wx-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Single-transaction scratch directory W^X DACL
	if err := setScratchDirDacl(tmpDir, profile.appContainer); err != nil {
		t.Fatalf("setScratchDirDacl failed: %v", err)
	}

	// Non-inheriting directory traverse grant
	if err := grantDirTraverse(tmpDir, profile.appContainer); err != nil {
		t.Fatalf("grantDirTraverse failed: %v", err)
	}

	// Clean up
	_ = revokePathAccess(tmpDir, profile.appContainer)
}

func TestWindowsDriver_LeaseJournalAndSweep(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	tmpDir, err := os.MkdirTemp("", "starkite-lease-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dummyLease := windowsLease{
		PID:             99999999, // Dead PID
		ProfileName:     "starkite.sbx.deadtest",
		AppContainerSID: "S-1-15-2-1",
		Paths:           []string{tmpDir},
		CreatedAt:       time.Now(),
	}

	if err := recordWindowsLease(dummyLease); err != nil {
		t.Fatalf("recordWindowsLease failed: %v", err)
	}

	// Sweep should clean up dead PID lease
	if err := sweepStaleWindowsLeases(); err != nil {
		t.Errorf("sweepStaleWindowsLeases failed: %v", err)
	}

	path := getLeaseFilePath()
	leases, _ := loadWindowsLeases(path)
	for _, l := range leases {
		if l.ProfileName == dummyLease.ProfileName {
			t.Errorf("expected dead lease %q to be swept, but still present", dummyLease.ProfileName)
		}
	}
}

func TestWindowsDriver_ExecResolveWindowsDirs(t *testing.T) {
	if len(defaultExecSearchDirs) == 0 {
		t.Errorf("expected non-empty defaultExecSearchDirs on Windows")
	}
	foundSystem32 := false
	for _, d := range defaultExecSearchDirs {
		if strings.Contains(strings.ToLower(d), "system32") {
			foundSystem32 = true
			break
		}
	}
	if !foundSystem32 {
		t.Errorf("expected defaultExecSearchDirs to include System32, got %v", defaultExecSearchDirs)
	}
}

func TestWindowsDriver_DriveRootDetection(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{`\`, true},
		{"C:", true},
		{`C:\`, true},
		{"C:/", true},
		{`C:\Windows`, false},
		{`C:\Users\alice`, false},
	}
	for _, tc := range tests {
		got := isDriveRoot(tc.path)
		if got != tc.want {
			t.Errorf("isDriveRoot(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

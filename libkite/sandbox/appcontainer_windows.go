//go:build windows

package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUserenv = windows.NewLazySystemDLL("userenv.dll")

	procCreateAppContainerProfile                 = modUserenv.NewProc("CreateAppContainerProfile")
	procDeleteAppContainerProfile                 = modUserenv.NewProc("DeleteAppContainerProfile")
	procDeriveAppContainerSidFromAppContainerName = modUserenv.NewProc("DeriveAppContainerSidFromAppContainerName")
)

// Well-known capability SID strings for Windows AppContainers.
const (
	sidCapabilityInternetClient       = "S-1-15-3-1" // outbound internet access
	sidCapabilityInternetClientServer = "S-1-15-3-2" // inbound/outbound internet and loopback
	sidCapabilityPrivateNetworkClient = "S-1-15-3-3" // private home/work intranet access
)

type securityCapabilities struct {
	AppContainerSid *windows.SID
	Capabilities    *windows.SIDAndAttributes
	CapabilityCount uint32
	Reserved        uint32
}

// appContainerProfile manages the lifecycle and security capabilities of an
// ephemeral Windows AppContainer security profile.
type appContainerProfile struct {
	name         string
	appContainer *windows.SID
	capabilities []windows.SIDAndAttributes
	secCaps      securityCapabilities
}

// newAppContainerProfile creates an ephemeral AppContainer profile with capability SIDs
// derived from spec.Network.
func newAppContainerProfile(network NetworkMode) (*appContainerProfile, error) {
	// Generate an ephemeral unique profile name: starkite.sbx.<random16hex>
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("sandbox: failed to generate profile uuid: %w", err)
	}
	profileName := "starkite.sbx." + hex.EncodeToString(randBytes)

	namePtr, err := windows.UTF16PtrFromString(profileName)
	if err != nil {
		return nil, err
	}
	dispPtr, _ := windows.UTF16PtrFromString("Starkite Sandbox")
	descPtr, _ := windows.UTF16PtrFromString("Starkite Ephemeral Sandbox Profile")

	var appContainerSID *windows.SID
	r1, _, _ := procCreateAppContainerProfile.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(dispPtr)),
		uintptr(unsafe.Pointer(descPtr)),
		0,
		0,
		uintptr(unsafe.Pointer(&appContainerSID)),
	)
	// r1 is an HRESULT; 0 is S_OK, 0x800700B7 is HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS)
	if r1 != 0 && r1 != 0x800700B7 {
		return nil, fmt.Errorf("sandbox: CreateAppContainerProfile (%s) failed: 0x%08x", profileName, uint32(r1))
	}

	if appContainerSID == nil {
		r1, _, _ := procDeriveAppContainerSidFromAppContainerName.Call(
			uintptr(unsafe.Pointer(namePtr)),
			uintptr(unsafe.Pointer(&appContainerSID)),
		)
		if r1 != 0 {
			return nil, fmt.Errorf("sandbox: DeriveAppContainerSidFromAppContainerName failed: 0x%08x", uint32(r1))
		}
	}

	p := &appContainerProfile{
		name:         profileName,
		appContainer: appContainerSID,
	}

	if err := p.configureCapabilities(network); err != nil {
		p.close()
		return nil, err
	}

	return p, nil
}

func (p *appContainerProfile) configureCapabilities(network NetworkMode) error {
	var sids []string
	switch network {
	case NetworkNone:
		// Zero capability SIDs: WFP blocks all AF_INET/AF_INET6 sockets with WSAEACCES (10013).
		sids = nil
	case NetworkHost:
		// Grant outbound Internet and Private network capabilities.
		sids = []string{sidCapabilityInternetClient, sidCapabilityPrivateNetworkClient}
	case NetworkLoopback:
		sids = []string{sidCapabilityInternetClientServer}
	}

	for _, s := range sids {
		sid, err := windows.StringToSid(s)
		if err != nil {
			return fmt.Errorf("sandbox: StringToSid(%s) failed: %w", s, err)
		}
		p.capabilities = append(p.capabilities, windows.SIDAndAttributes{
			Sid:        sid,
			Attributes: windows.SE_GROUP_ENABLED,
		})
	}

	p.secCaps = securityCapabilities{
		AppContainerSid: p.appContainer,
	}
	if len(p.capabilities) > 0 {
		p.secCaps.Capabilities = &p.capabilities[0]
		p.secCaps.CapabilityCount = uint32(len(p.capabilities))
	}

	return nil
}

// securityCapabilitiesPtr returns the pointer to securityCapabilities struct
// formatted for PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES.
func (p *appContainerProfile) securityCapabilitiesPtr() *securityCapabilities {
	if p == nil || p.appContainer == nil {
		return nil
	}
	return &p.secCaps
}

// deleteAppContainerProfileByName deletes an AppContainer profile by name from the host registry.
func deleteAppContainerProfileByName(name string) error {
	if name == "" {
		return nil
	}
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r1, _, _ := procDeleteAppContainerProfile.Call(uintptr(unsafe.Pointer(namePtr)))
	if r1 != 0 {
		return fmt.Errorf("sandbox: DeleteAppContainerProfile(%s) failed: 0x%08x", name, uint32(r1))
	}
	return nil
}

// close deletes the ephemeral AppContainer profile from the host registry and frees its Win32 SID.
func (p *appContainerProfile) close() {
	if p.name != "" {
		_ = deleteAppContainerProfileByName(p.name)
		p.name = ""
	}
	if p.appContainer != nil {
		_ = windows.FreeSid(p.appContainer)
		p.appContainer = nil
	}
}

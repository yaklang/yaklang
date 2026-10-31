//go:build windows

package trafficfixture

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privateWorkspacePrincipals() ([]*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	return uniqueWorkspacePrincipals(user.User.Sid, system)
}

func uniqueWorkspacePrincipals(principals ...*windows.SID) ([]*windows.SID, error) {
	var unique []*windows.SID
	for _, sid := range principals {
		if sid == nil || !sid.IsValid() {
			return nil, fmt.Errorf("trafficfixture: invalid private workspace principal")
		}
		found := false
		for _, previous := range unique {
			found = found || sid.Equals(previous)
		}
		if !found {
			unique = append(unique, sid)
		}
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("trafficfixture: private workspace has no principal")
	}
	return unique, nil
}

func privateWorkspaceDescriptor(principals []*windows.SID) (*windows.SECURITY_DESCRIPTOR, error) {
	principals, err := uniqueWorkspacePrincipals(principals...)
	if err != nil {
		return nil, err
	}
	var sddl strings.Builder
	sddl.WriteString("D:P")
	for _, sid := range principals {
		// OI/CI propagate the same two-principal policy to newly created
		// files and subdirectories. No payload is written before this DACL.
		sddl.WriteString("(A;OICI;FA;;;")
		sddl.WriteString(sid.String())
		sddl.WriteByte(')')
	}
	return windows.SecurityDescriptorFromString(sddl.String())
}

func preparePrivateWorkspace(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("trafficfixture: private workspace requires a nonsymlink directory: %s", directory)
	}
	principals, err := privateWorkspacePrincipals()
	if err != nil {
		return err
	}
	sd, err := privateWorkspaceDescriptor(principals)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("trafficfixture: private workspace descriptor has no DACL: %v", err)
	}
	err = windows.SetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
	runtime.KeepAlive(sd)
	if err != nil {
		return err
	}
	return checkPrivateWorkspacePermissions(directory, true)
}

func checkPrivateWorkspacePermissions(name string, root bool) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() || root && !info.IsDir() {
		return fmt.Errorf("trafficfixture: invalid private workspace object: %s", name)
	}
	principals, err := privateWorkspacePrincipals()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return validatePrivateWorkspaceDACL(sd, principals, root, info.IsDir())
}

// FileMode on Windows only reflects READONLY and cannot prove privacy. Compare
// binary ACE trustees and real access masks instead of rendered SDDL aliases.
func validatePrivateWorkspaceDACL(sd *windows.SECURITY_DESCRIPTOR, principals []*windows.SID, root, directory bool) error {
	if sd == nil || !sd.IsValid() {
		return fmt.Errorf("trafficfixture: invalid private workspace security descriptor")
	}
	principals, err := uniqueWorkspacePrincipals(principals...)
	if err != nil {
		return err
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if root && control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("trafficfixture: workspace root DACL is not protected")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil || int(dacl.AceCount) != len(principals) {
		return fmt.Errorf("trafficfixture: workspace requires a non-NULL DACL with exact private principals")
	}
	seen := make([]bool, len(principals))
	const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		wantFlags := uint8(0)
		if directory {
			wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			ace.Header.AceFlags & ^uint8(windows.INHERITED_ACE) != wantFlags ||
			root && ace.Header.AceFlags&windows.INHERITED_ACE != 0 ||
			ace.Mask != windows.GENERIC_ALL && ace.Mask != fileAllAccess {
			return fmt.Errorf("trafficfixture: workspace ACE %d has invalid type, inheritance or access", i)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		found := false
		for j, principal := range principals {
			if !seen[j] && sid.Equals(principal) {
				seen[j], found = true, true
				break
			}
		}
		if !found {
			return fmt.Errorf("trafficfixture: workspace ACE %d grants an unexpected or duplicate principal", i)
		}
	}
	runtime.KeepAlive(sd)
	return nil
}

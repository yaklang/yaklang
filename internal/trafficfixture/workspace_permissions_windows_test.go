//go:build windows

package trafficfixture

import (
	"fmt"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateWorkspaceWindowsDACLRejectsBroaderOrNoninheritedAccess(t *testing.T) {
	user, err := windows.StringToSid("S-1-5-21-111-222-333-1001")
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source    string
		root, directory bool
		valid           bool
	}{
		{"private-root", "D:P(A;OICI;FA;;;%s)(A;OICI;FA;;;SY)", true, true, true},
		{"inherited-file", "D:(A;ID;FA;;;%s)(A;ID;FA;;;SY)", false, false, true},
		{"inherited-directory", "D:(A;OICIID;FA;;;%s)(A;OICIID;FA;;;SY)", false, true, true},
		{"unprotected-root", "D:(A;OICI;FA;;;%s)(A;OICI;FA;;;SY)", true, true, false},
		{"everyone", "D:P(A;OICI;FA;;;%s)(A;OICI;FA;;;WD)", true, true, false},
		{"extra-principal", "D:P(A;OICI;FA;;;%s)(A;OICI;FA;;;SY)(A;OICI;FR;;;WD)", true, true, false},
		{"duplicate-user", "D:P(A;OICI;FA;;;%s)(A;OICI;FA;;;%s)", true, true, false},
		{"missing-system", "D:P(A;OICI;FA;;;%s)", true, true, false},
		{"inherit-only", "D:P(A;OICIIO;FA;;;%s)(A;OICI;FA;;;SY)", true, true, false},
		{"no-inheritance", "D:P(A;;FA;;;%s)(A;;FA;;;SY)", true, true, false},
		{"insufficient-access", "D:P(A;OICI;FR;;;%s)(A;OICI;FA;;;SY)", true, true, false},
		{"deny-instead-of-allow", "D:P(D;OICI;FA;;;%s)(A;OICI;FA;;;SY)", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var source string
			if tc.name == "duplicate-user" {
				source = fmt.Sprintf(tc.source, user.String(), user.String())
			} else {
				source = fmt.Sprintf(tc.source, user.String())
			}
			sd, err := windows.SecurityDescriptorFromString(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := validatePrivateWorkspaceDACL(sd, []*windows.SID{user, system}, tc.root, tc.directory); (err == nil) != tc.valid {
				t.Fatalf("DACL acceptance=%v, want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
	for _, sddl := range []string{"O:SY", "D:P"} {
		sd, err := windows.SecurityDescriptorFromString(sddl)
		if err != nil {
			t.Fatal(err)
		}
		if err := validatePrivateWorkspaceDACL(sd, []*windows.SID{user, system}, false, false); err == nil {
			t.Fatal("accepted absent/empty DACL")
		}
	}
	null, err := windows.NewSecurityDescriptor()
	if err != nil {
		t.Fatal(err)
	}
	if err := null.SetDACL(nil, true, false); err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateWorkspaceDACL(null, []*windows.SID{user, system}, false, false); err == nil {
		t.Fatal("accepted permissive NULL DACL")
	}
}

func TestPrivateWorkspaceWindowsSystemPrincipalIsNotDuplicated(t *testing.T) {
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	principals, err := uniqueWorkspacePrincipals(system, system)
	if err != nil || len(principals) != 1 {
		t.Fatal("SYSTEM user and SYSTEM service must be one principal", err)
	}
	sd, err := privateWorkspaceDescriptor(principals)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateWorkspaceDACL(sd, []*windows.SID{system, system}, true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := uniqueWorkspacePrincipals(nil); err == nil {
		t.Fatal("accepted an unknown principal")
	}
}

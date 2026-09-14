//go:build windows

package engineendpoint

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNamedPipeACLContainsOnlyCurrentUserAndSystem(t *testing.T) {
	transport, endpoint := testEndpoint(t)
	l, err := Listen(transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	accepted := make(chan net.Conn, 1)
	go func() { c, _ := l.Accept(); accepted <- c }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := DialContext(ctx, transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	if server == nil {
		t.Fatal("accept failed")
	}
	defer server.Close()
	handle := windows.Handle(client.(interface{ Fd() uintptr }).Fd())
	sd, err := windows.GetSecurityInfo(handle, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePrivatePipeDACL(sd, user.User.Sid); err != nil {
		t.Fatalf("invalid private pipe DACL: %v; SDDL=%s", err, sd.String())
	}
	label, err := windows.GetSecurityInfo(handle, windows.SE_KERNEL_OBJECT, windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(label.String(), "(ML;;NW;;;ME)") {
		t.Fatalf("pipe must explicitly accept ordinary desktop clients even when created elevated: %s", label.String())
	}
}

func validatePrivatePipeDACL(sd *windows.SECURITY_DESCRIPTOR, user *windows.SID) error {
	if sd == nil || !sd.IsValid() || user == nil || !user.IsValid() {
		return fmt.Errorf("invalid security descriptor or user SID")
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("DACL must be protected from inherited permissions")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if dacl == nil || dacl.AceCount != 2 {
		return fmt.Errorf("DACL must contain exactly two explicit grants")
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	// SDDL may abbreviate the same SID as LA (the hosted runner's built-in
	// administrator), SY, etc. Compare the binary ACE trustees, not display text.
	expected := []*windows.SID{user, system}
	seen := [2]bool{}
	// Named pipes map GENERIC_ALL to WinNT's FILE_ALL_ACCESS at creation time.
	const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 {
			return fmt.Errorf("ACE %d is not an explicit, non-inherited allow entry", i)
		}
		if ace.Mask != windows.GENERIC_ALL && ace.Mask != fileAllAccess {
			return fmt.Errorf("ACE %d has unexpected access mask %#x", i, ace.Mask)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		matched := false
		for j, want := range expected {
			if !seen[j] && sid.Equals(want) {
				seen[j], matched = true, true
				break
			}
		}
		if !matched {
			return fmt.Errorf("ACE %d grants an unexpected or duplicate SID %s", i, sid.String())
		}
	}
	return nil
}

func TestNamedPipeACLValidation(t *testing.T) {
	const ordinarySID = "S-1-5-21-111-222-333-1001"
	const otherSID = "S-1-5-21-111-222-333-1002"
	// Resolve LA using Windows, without creating or impersonating an account.
	// The hosted runner's built-in administrator is rendered as this alias.
	adminSD, err := windows.SecurityDescriptorFromString("O:LA")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := adminSD.Owner()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, sddl, user string
		valid            bool
	}{
		{"ordinary-user", "D:P(A;;GA;;;SY)(A;;GA;;;" + ordinarySID + ")", ordinarySID, true},
		{"admin-alias", "D:P(A;;FA;;;SY)(A;;FA;;;LA)", admin.String(), true},
		{"admin-numeric-sid", "D:P(A;;FA;;;SY)(A;;FA;;;" + admin.String() + ")", admin.String(), true},
		{"reordered", "D:P(A;;GA;;;" + ordinarySID + ")(A;;GA;;;S-1-5-18)", ordinarySID, true},
		{"system-process", "D:P(A;;GA;;;SY)(A;;GA;;;SY)", "S-1-5-18", true},
		{"everyone", "D:P(A;;FA;;;SY)(A;;FA;;;WD)", ordinarySID, false},
		{"authenticated-users", "D:P(A;;FA;;;SY)(A;;FA;;;AU)", ordinarySID, false},
		{"builtin-users", "D:P(A;;FA;;;SY)(A;;FA;;;BU)", ordinarySID, false},
		{"admin-group", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", ordinarySID, false},
		{"wrong-user", "D:P(A;;GA;;;SY)(A;;GA;;;" + otherSID + ")", ordinarySID, false},
		{"alias-is-not-current-user", "D:P(A;;FA;;;SY)(A;;FA;;;LA)", ordinarySID, false},
		{"extra-user", "D:P(A;;GA;;;SY)(A;;GA;;;" + ordinarySID + ")(A;;GA;;;" + otherSID + ")", ordinarySID, false},
		{"duplicate-user", "D:P(A;;GA;;;" + ordinarySID + ")(A;;GA;;;" + ordinarySID + ")", ordinarySID, false},
		{"missing-system", "D:P(A;;GA;;;" + ordinarySID + ")", ordinarySID, false},
		{"deny-user", "D:P(A;;GA;;;SY)(D;;GA;;;" + ordinarySID + ")", ordinarySID, false},
		{"inherit-only", "D:P(A;;GA;;;SY)(A;IO;GA;;;" + ordinarySID + ")", ordinarySID, false},
		{"inherited", "D:P(A;;GA;;;SY)(A;ID;GA;;;" + ordinarySID + ")", ordinarySID, false},
		{"read-only", "D:P(A;;GA;;;SY)(A;;GR;;;" + ordinarySID + ")", ordinarySID, false},
		{"unprotected", "D:(A;;GA;;;SY)(A;;GA;;;" + ordinarySID + ")", ordinarySID, false},
		{"null-dacl", "D:NO_ACCESS_CONTROL", ordinarySID, false},
		{"empty-dacl", "D:P", ordinarySID, false},
		{"absent-dacl", "O:SY", ordinarySID, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			user, err := windows.StringToSid(tc.user)
			if err != nil {
				t.Fatal(err)
			}
			if err := validatePrivatePipeDACL(sd, user); (err == nil) != tc.valid {
				t.Fatalf("valid=%t, error=%v; SDDL=%s", tc.valid, err, sd.String())
			}
		})
	}
}

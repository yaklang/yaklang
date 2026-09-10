//go:build windows

package engineendpoint

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

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
	actual := sd.String()
	if strings.Contains(actual, ";;;WD)") || strings.Contains(actual, ";;;AU)") || strings.Contains(actual, ";;;BU)") {
		t.Fatalf("pipe grants access to other local users: %s", actual)
	}
	if !strings.Contains(actual, ";;;SY)") || !strings.Contains(actual, ";;;"+user.User.Sid.String()+")") {
		t.Fatalf("missing explicit current-user/system ACL: %s", actual)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl.AceCount != 2 {
		t.Fatalf("unexpected extra ACL entries: %s", actual)
	}
	label, err := windows.GetSecurityInfo(handle, windows.SE_KERNEL_OBJECT, windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(label.String(), "(ML;;NW;;;ME)") {
		t.Fatalf("pipe must explicitly accept ordinary desktop clients even when created elevated: %s", label.String())
	}
}

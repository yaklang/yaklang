package core

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/netutil"
)

func TestInterface(t *testing.T) {
	iface, _, err := GetInterfaceInWindows()
	if err != nil {
		t.Error(err)
	}
	t.Log(iface)
}

func TestPublicInterface(t *testing.T) {
	ifaces := GetIfaceIPAddress()
	t.Log(ifaces)
	t.Log(getPublicRoute())
}

func getPublicRoute() (*net.Interface, net.IP, net.IP, error) {
	iface, gw, ip, err := netutil.Route(3*time.Second, "8.8.8.8")
	if err != nil {
		return nil, nil, nil, err
	}
	fmt.Println(iface.Name)
	return iface, gw, ip, nil
}

func TestFindDevs(t *testing.T) {
	result, _ := GetIfaceIPAddressInWindows()
	t.Log(result)
}

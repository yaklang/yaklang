package pcapfix

import (
	"github.com/google/gopacket/pcap"
	"runtime"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils/netutil"
	"yaklang/common/utils/permutil"
)

func IsPrivilegedForNetRaw() bool {
	switch runtime.GOOS {
	case "windows":
		if permutil.IAmAdmin() {
			return true
		} else {
			return false
		}
	default:
		i, err := netutil.FindInterfaceByIP("127.0.0.1")
		if err != nil {
			log.Errorf("cannot found net.Interface by ip: %s", err)
			return false
		}
		handler, err := pcap.OpenLive(i.Name, 65536, true, 5*time.Second)
		if err != nil {
			return false
		}
		handler.Close()
		return true
	}
}

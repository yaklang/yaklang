package pingutil

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
	"time"
)

func TestPcapxPing(t *testing.T) {
	targets := utils.ParseStringToHosts(`47.52.100.1/20`)
	for _, ip := range targets {
		ip := ip
		go func() {
			result, err := PcapxPing(ip, NewPingConfig())
			if err != nil {
				t.Fatal(err)
			}
			if result.Ok {
				fmt.Println(result.IP + " is alive")
			}
		}()
	}
	time.Sleep(30 * time.Second)
}

package yaklib

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

func Test_pingScan(t *testing.T) {
	for res := range _pingScan("192.168.3.3/24", _pingConfigOpt_concurrent(50)) {
		if res.Ok {
			log.Infof("ping %s success: %v", res.IP, res.RTT)
		} else {
			log.Infof("ping %s failed: %s", res.IP, res.Reason)
		}
	}
	log.Infof("done")
}

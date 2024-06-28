package tools

import (
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
	"time"
)

func TestHostPortFilter(t *testing.T) {
	filter := utils.NewHostsFilter("47.52.100.1/24")
	filter.Add("127.0.0.1/24")

	if filter.Contains("47.52.100.1") || filter.Contains("127.0.0.23") {
		return
	}

	filter.Add("27.12.5.1")
	if filter.Contains("27.12.5.1") {
		return
	}

	panic(1)
}

func TestBasicSynScanIntegrate(t *testing.T) {
	//log.SetLevel(log.DebugLevel)
	config := &_yakPortScanConfig{
		waiting:           10 * time.Second,
		rateLimitDelayMs:  1,
		rateLimitDelayGap: 5,
		excludePorts: filter.NewFilter(),
		//netInterface:      "\\Device\\NPF_{6E6F3FC9-4678-48E2-B746-C5DEEFE6CDF0}",
		//netInterface: "WLAN 4",
		//netInterface: "Radmin VPN",
	}

	res, err := _synScanDo(hostsToChan("192.168.3.2"), "21,22,23,80,443,U:161", config)
	if err != nil {
		return
	}
	for result := range res {
		result.Show()
	}
}

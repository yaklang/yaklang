package tools

import (
	"testing"

	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils"
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
	t.Skip("跳过测试：依赖外部IP 124.222.42.210，不符合测试不外连的原则")

	res, err := _scanx("124.222.42.210", "80", synscanx.WithWaiting(10), synscanx.WithIface("Radmin VPN"))
	if err != nil {
		return
	}
	for result := range res {
		result.Show()
	}
}

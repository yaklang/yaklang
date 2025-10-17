package tools

import (
	"testing"
	"time"
)

func TestBasicFinScanIntegrate(t *testing.T) {
	t.Skip("跳过测试：依赖外部IP 124.222.42.210，不符合测试不外连的原则")

	//log.SetLevel(log.DebugLevel)
	config := &_yakFinPortScanConfig{
		waiting:           2 * time.Second, // 将等待时间从10秒减少到2秒
		rateLimitDelayMs:  1,
		rateLimitDelayGap: 5,
	}

	res, err := _finscanDo(hostsToChan("124.222.42.210"), "81", config)
	if err != nil {
		return
	}
	for result := range res {
		result.Show()
	}
}

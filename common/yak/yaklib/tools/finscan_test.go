package tools

import (
	"testing"
	"time"
)

func TestBasicFinScanIntegrate(t *testing.T) {
	//log.SetLevel(log.DebugLevel)
	config := &_yakFinPortScanConfig{
		waiting:           10 * time.Second,
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

package yakscript

import (
	"context"
	"strconv"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TickerRiskCountFeedback periodically updates risk count to the virtual client (project DB).
func TickerRiskCountFeedback(ctx context.Context, tickerTime time.Duration, runtimeId string, yakClient *yaklib.YakitClient, projectDB *gorm.DB) {
	ticker := time.NewTicker(tickerTime)
	go func() {
		defer ticker.Stop()
		lastCount := 0
		feedbackRiskCount := func() {
			currentCount, err := yakit.CountRiskByRuntimeId(projectDB, runtimeId)
			if err != nil {
				log.Errorf("count risk failed: %v", err)
			} else if lastCount != currentCount {
				yakClient.Output(&yaklib.YakitStatusCard{
					Id: "漏洞/风险/指纹", Data: strconv.Itoa(currentCount), Tags: nil,
				})
			}
			lastCount = currentCount
		}
		for {
			select {
			case <-ctx.Done():
				feedbackRiskCount()
				return
			case <-ticker.C:
				feedbackRiskCount()
			}
		}
	}()
}

// ForceRiskCountFeedback sends a final risk count update.
func ForceRiskCountFeedback(runtimeId string, yakClient *yaklib.YakitClient, projectDB *gorm.DB) (int, error) {
	riskCount, err := yakit.CountRiskByRuntimeId(projectDB, runtimeId)
	if err != nil {
		log.Errorf("count risk failed: %v", err)
	} else {
		yakClient.Output(&yaklib.YakitStatusCard{ // card
			Id: "漏洞/风险/指纹", Data: strconv.Itoa(riskCount), Tags: nil,
		})
	}
	return riskCount, err
}

func printStackOnRecover() {
	if err := recover(); err != nil {
		log.Warn(err)
		utils.PrintCurrentGoroutineRuntimeStack()
	}
}

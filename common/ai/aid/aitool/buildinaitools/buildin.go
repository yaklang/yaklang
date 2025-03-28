package buildinaitools

import (
	"io"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func GetBasicBuildInTools() []*aitool.Tool {
	nowTime, err := aitool.New(
		"now",
		aitool.WithDescription("get current time"),
		aitool.WithStringParam(
			"timezone",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("timezone for now, like 'Asia/Shanghai' or 'UTC' ... "),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return time.Now().String(), nil
		}),
	)
	if err != nil {
		log.Errorf("create now tool: %v", err)
	}

	tools := []*aitool.Tool{nowTime}
	return lo.Filter(tools, func(item *aitool.Tool, index int) bool {
		if utils.IsNil(item) {
			log.Errorf("tool is nil")
			return false
		}
		return true
	})
}

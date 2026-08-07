package browsercrypto

import (
	"errors"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/browsertools"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const BrowserTransformAgentContractVersion = 1

type Config struct {
	DeviceID string
	Target   browsertools.Target
	Query    string
}

func normalizedParamKey(value string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(
		strings.ToLower(strings.TrimSpace(value)),
	)
}

func ParseConfig(items []*ypb.ExecParamItem) (Config, error) {
	var config Config
	for _, item := range items {
		if item == nil {
			continue
		}
		value := strings.TrimSpace(item.GetValue())
		switch normalizedParamKey(item.GetKey()) {
		case "deviceid":
			config.DeviceID = value
		case "tabid":
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				return config, errors.New("browser crypto analysis requires a positive tab_id")
			}
			config.Target.TabID = parsed
		case "frameid":
			if value == "" {
				continue
			}
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				return config, errors.New("browser crypto analysis frame_id must be zero or greater")
			}
			config.Target.FrameID = parsed
		case "documentid":
			config.Target.DocumentID = value
		case "query":
			config.Query = strings.TrimSpace(item.GetValue())
		}
	}
	if config.DeviceID == "" {
		return config, errors.New("browser crypto analysis requires device_id")
	}
	if config.Target.TabID < 1 {
		return config, errors.New("browser crypto analysis requires tab_id")
	}
	if config.Query == "" {
		config.Query = "Analyze the frontend cryptography flow recorded for the current page, then propose and validate a browser Transform Profile that accepts a plaintext request."
	}
	return config, nil
}

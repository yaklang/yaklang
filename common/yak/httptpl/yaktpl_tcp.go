package httptpl

import (
	"github.com/yaklang/yaklang/common/log"
	"strings"
)

type TCPRequestBulk struct {
	NetworkBulkConfig *YakNetworkBulkConfig
}

type YakNetworkBulkConfig struct {
	Inputs   []*YakTcpInput
	Hosts    []string
	ReadSize int

	Matcher   *YakMatcher
	Extractor []*YakExtractor
}

type YakTcpInput struct {
	// data / read(int) / type: hex
	Data string
	Read int
	Type string
}

type YakTcpHosts struct {
}

func (y *YakTcpInput) BuildPayload(vars map[string]any) {
	var data = y.Data
	if strings.Contains(y.Data, `{{`) && strings.Contains(y.Data, "}}") {
		tags := ParseNucleiTag(y.Data)
		sandbox := NewNucleiDSLYakSandbox()
		dataRaw, ok, err := ExecuteNucleiTags(tags, sandbox, vars)
		if !ok || err != nil {
			log.Warnf(`YakTcpInput.Execute.ExecuteNucleiTags failed: %s`, err)
		} else {
			data = dataRaw
		}
	}
	_ = data
}

package httptpl

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
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

	ReverseConnectionNeed bool
}

type YakTcpInput struct {
	// data / read(int) / type: hex
	Data string
	Read int
	Type string
}

type YakTcpHosts struct{}

func (y *YakTcpInput) BuildPayload(vars map[string]any) {
	data := y.Data
	if strings.Contains(y.Data, `{{`) && strings.Contains(y.Data, "}}") {
		result, err := ExecNucleiDSL(y.Data, vars)
		if err != nil {
			log.Warnf(`YakTcpInput.Execute.ExecuteNucleiTags failed: %s`, err)
		} else {
			data = toString(result)
		}
	}
	_ = data
}

package aispec

import (
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"io"
)

type Chatter interface {
	Chat(string, ...Function) (string, error)
	ChatEx([]ChatDetail, ...Function) ([]ChatChoice, error)
	ChatStream(string) (io.Reader, error)
}

type FunctionCaller interface {
	ExtractData(data string, desc string, fields map[string]any) (map[string]any, error)
}

type Configurable interface {
	LoadOption(opt ...AIConfigOption)
	BuildHTTPOptions() ([]poc.PocConfigOption, error)
}

type AIGateway interface {
	Chatter
	FunctionCaller
	Configurable
}

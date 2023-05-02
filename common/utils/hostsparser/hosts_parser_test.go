package hostsparser

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"testing"
	"github.com/yaklang/yaklang/common/log"
)

func TestHostsParser(t *testing.T) {

	for _, raw := range []struct {
		input       string
		size        int
		contains    []string
		notContains []string
	}{
		{
			input: "47.52.100.105/24", size: 256,
			contains:    []string{"47.52.100.105", "47.52.100.5", "47.52.100.0", "47.52.100.255"},
			notContains: []string{"47.52.100", "baidu.com", "47.52.100.-1", "47.52.100.256"},
		},
		{input: "47.52.100.104", size: 1},
		{input: "47.52.100.104-222", size: 222 - 104},
		{input: "baidu.com", size: 1},
		{input: "47.52.100.105/24,baidu.com", size: 257},
	} {
		parser := NewHostsParser(context.Background(), raw.input)
		log.Infof("%s has %v", raw.input, parser.Size())
		spew.Dump(parser.Blocks)
		for host := range parser.Hosts() {
			spew.Dump(host)
		}
	}
}

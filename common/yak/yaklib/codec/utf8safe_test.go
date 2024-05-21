package codec

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"testing"
)

func TestUTF8Safe(t *testing.T) {
	for _, c := range []struct {
		Input   string
		Contain []string
	}{
		{
			Input: "\x00E\x00S\x00你好", Contain: []string{"你好", "E", "S"},
		},
		{
			Input: "\xc4\xe3\xba\xc3", Contain: []string{`\xc4\xe3\xba\xc3`},
		},
	} {
		ret := UTF8SafeEscape(c.Input)
		log.Infof("UTF8SafeEscape(%#v) -> %#v", c.Input, ret)
		fmt.Println(ret)
		for _, s := range c.Contain {
			if !strings.Contains(ret, s) {
				t.Fatalf("expect: %#v in %#v", s, ret)
			}
		}
	}
}

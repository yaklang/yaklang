package poc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestDoWithAutoHTTPS(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	thisTest := func() {
		host, port := utils.DebugMockHTTPS([]byte(`HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n` + strings.Repeat("a", 4096)))
		rspInst, _, err := Do("GET", fmt.Sprintf("https://%s:%d", host, port))
		_ = rspInst
		if err != nil {
			t.Fatal(err)
		}
	}

	err := utils.CallWithCtx(ctx, thisTest)
	if err != nil {
		t.Fatal(err)
	}
}

package netstackvm

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"testing"
	"time"
)

func TestNetstackMonitor(t *testing.T) {
	if os.Getenv("NETSTACKVM_HOST_TESTS") != "1" {
		t.Skip("manual host-network test; set NETSTACKVM_HOST_TESTS=1 explicitly")
	}
	if utils.InGithubActions() {
		t.Skip("skip in github actions")
	}
	m, err := StartTargetMonitor()
	require.NoError(t, err)

	go func() {
		for {
			time.Sleep(5 * time.Second)
			spew.Dump(m.GetAliveIP())
			spew.Dump(m.GetAliveDomain())
		}
	}()

	select {}
}

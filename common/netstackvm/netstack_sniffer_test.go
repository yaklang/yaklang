package netstackvm

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
	"time"
)

func TestNetstackMonitor(t *testing.T) {
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

func TestNetstackCache(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in github actions")
	}
	m, err := StartNetReserveStackDNSCache(context.Background())
	require.NoError(t, err)

	go func() {
		for {
			time.Sleep(5 * time.Second)
			domainList := m.ReserveResolve("183.2.172.177")
			if len(domainList) > 0 {
				fmt.Printf("found domain %v\n", domainList)
			}
		}
	}()

	select {}
}

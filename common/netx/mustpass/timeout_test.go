package mustpass

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

func TestProxyFastTimeout(t *testing.T) {
	var (
		proxyCheckErr error
	)

	t.Run("http proxy timeout", func(t *testing.T) {
		err := utils.CallWithTimeout(2, func() {
			_, proxyCheckErr = netx.ProxyCheck("http://127.0.0.1:65534", 1*time.Second)
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("should not call timeout, but timeout: %#v", err)
			} else {
				t.Fatalf("Unexpected error: %#v", err)
			}
		}
		require.Error(t, proxyCheckErr)
	})

	t.Run("socks proxy timeout", func(t *testing.T) {
		err := utils.CallWithTimeout(2, func() {
			_, proxyCheckErr = netx.ProxyCheck("socks5://127.0.0.1:65534", 1*time.Second)
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("should not call timeout, but timeout: %#v", err)
			} else {
				t.Fatalf("Unexpected error: %#v", err)
			}
		}
		require.Error(t, proxyCheckErr)
	})
}

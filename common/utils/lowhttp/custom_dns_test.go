package lowhttp_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestLowHTTPConfiguredDNSOverridesSystemResolver(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	domain := strings.ToLower(utils.RandStringBytes(16)) + ".invalid"
	_, targetPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("custom-dns-ok"))
	})

	// Seed the process-wide DNS cache with an address from another resolver.
	// A request-scoped resolver must not inherit that unrelated answer.
	staleDNSServer := facades.MockDNSServer(ctx, domain, utils.GetRandomAvailableTCPPort(), func(record, domain string) string {
		return "127.0.0.2"
	})
	require.NotEmpty(t, staleDNSServer)
	require.Equal(t, "127.0.0.2", netx.LookupFirst(
		domain,
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(staleDNSServer),
	))

	var dnsQueries atomic.Int32
	dnsServer := facades.MockDNSServer(ctx, domain, utils.GetRandomAvailableTCPPort(), func(record, domain string) string {
		if record == "A" {
			dnsQueries.Add(1)
		}
		return "127.0.0.1"
	})
	require.NotEmpty(t, dnsServer)

	request := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s:%d\r\nConnection: close\r\n\r\n", domain, targetPort))
	response, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes(request),
		lowhttp.WithDNSServers([]string{dnsServer}),
		lowhttp.WithTimeout(5*time.Second),
	)
	require.NoError(t, err)
	require.Contains(t, string(response.RawPacket), "custom-dns-ok")
	require.GreaterOrEqual(t, dnsQueries.Load(), int32(1), "configured DNS must be queried instead of reusing another resolver's cache")
}

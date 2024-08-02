package facades

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"strings"
	"testing"
)

func TestDNSServer(t *testing.T) {
	token := strings.ToLower(utils.RandStringBytes(10))

	baseDomain := utils.RandSample(10) + ".com"
	baseDomain = strings.ToLower(baseDomain)
	randPort := utils.GetRandomAvailableTCPPort()
	randIp := utils.Uint32ToIPv4(rand.Uint32()).To4().String()
	dnsServer, err := NewDNSServer(baseDomain, randIp, "0.0.0.0", randPort)
	if err != nil {
		t.Fatal(err)
	}
	dnsServer.SetCallback(func(i *VisitorLog) {
		spew.Dump(i)
	})
	go func() {
		dnsServer.Serve(utils.TimeoutContextSeconds(10))
	}()
	addr := utils.HostPort("127.0.0.1", randPort)
	err = utils.WaitConnect(addr, 5)
	if err != nil {
		t.Fatal(err)
	}

	result := netx.LookupFirst(
		baseDomain,
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr),
	)
	assert.Equal(t, result, randIp)

	result = netx.LookupFirst(
		token+"."+baseDomain,
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr),
	)
	assert.Equal(t, result, randIp)

	result = netx.LookupFirst(
		token+"."+strings.ToLower(utils.RandStringBytes(10))+".bb.com",
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr),
	)
	assert.Equal(t, result, randIp)
}

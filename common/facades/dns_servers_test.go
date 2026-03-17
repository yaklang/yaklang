package facades

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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

	checkSuffix := strings.ToLower(utils.RandStringBytes(10)) + ".xyz"
	checkToken := ""
	dnsServer.SetCallback(func(i *VisitorLog) {
		if strings.HasSuffix(strings.Trim(i.GetDomain(), "."), checkSuffix) {
			anyToken, ok := i.Details["token"]
			if ok {
				checkToken = codec.AnyToString(anyToken)
			}
		}
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

	anyDomainToken := strings.ToLower(utils.RandStringBytes(10))
	result = netx.LookupFirst(
		anyDomainToken+"."+checkSuffix,
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr),
	)
	assert.Equal(t, result, randIp)
	assert.Equal(t, checkToken, anyDomainToken)
}

// TestDNSServer_CustomDomain 测试自定义域名场景。
// 使用非标准 TLD（未在 domainextractor 中注册）的域名，其子域可随机变化（如 xxx.root.yyy），
// 应正确设置 token 并触发 callback。当 ExtractRootDomain 无法识别时，应使用配置的 domain 进行后缀匹配。
func TestDNSServer_CustomDomain(t *testing.T) {
	// 生成非标准 TLD 的根域，确保 ExtractRootDomain 无法识别（会回退到完整域名，导致 suffix 匹配失败）
	baseDomain := strings.ToLower(utils.RandStringBytes(6)) + "." + strings.ToLower(utils.RandStringBytes(6))
	randPort := utils.GetRandomAvailableTCPPort()
	randIp := utils.Uint32ToIPv4(rand.Uint32()).To4().String()
	dnsServer, err := NewDNSServer(baseDomain, randIp, "0.0.0.0", randPort)
	if err != nil {
		t.Fatal(err)
	}

	receivedToken := ""
	dnsServer.SetCallback(func(i *VisitorLog) {
		tok, ok := i.Details["token"]
		if ok {
			receivedToken = codec.AnyToString(tok)
		}
	})

	go func() {
		dnsServer.Serve(utils.TimeoutContextSeconds(10))
	}()
	addr := utils.HostPort("127.0.0.1", randPort)
	if err := utils.WaitConnect(addr, 5); err != nil {
		t.Fatal(err)
	}

	// 随机子域 + baseDomain，应能解析并设置 token
	token := strings.ToLower(utils.RandStringBytes(8))
	queryDomain := token + "." + baseDomain

	result := netx.LookupFirst(
		queryDomain,
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr),
	)
	assert.Equal(t, randIp, result, "DNS lookup should succeed")

	// 给 callback 一点时间执行
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, token, receivedToken,
		"callback should receive token for custom domain subdomain query")
}

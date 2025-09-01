package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/tlsutils"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type GetawayClient struct {
	valid              bool
	functionCallHandle func(msg string) (map[string]any, error)
}

func (g *GetawayClient) SupportedStructuredStream() bool {
	return false
}

func (g *GetawayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return nil, nil
}

func (g *GetawayClient) StructuredStream(s string, function ...any) (chan *aispec.StructuredData, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GetawayClient) CheckValid() error {
	if g.valid {
		return nil
	}
	return errors.New("invalid")
}

func (g *GetawayClient) Chat(s string, function ...any) (string, error) {
	if g.valid {
		return "ok", nil
	}
	return "", errors.New("invalid")
}

func (g *GetawayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return g.functionCallHandle(msg)
}

func (g *GetawayClient) ChatStream(s string) (io.Reader, error) {
	return nil, nil
}

func (g *GetawayClient) LoadOption(opt ...aispec.AIConfigOption) {
}

func (g *GetawayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	return nil, nil
}

func TestAiApiPriority(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}
	config_bak, _ := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	defer func() {
		client.SetGlobalNetworkConfig(context.Background(), config_bak)
	}()
	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	config.AiApiPriority = []string{"test", "test1", "test2"}
	var ok, test1, test2 bool
	aispec.Register("test", func() aispec.AIClient {
		ok = true
		return nil
	})
	aispec.Register("test1", func() aispec.AIClient {
		test1 = true
		return &GetawayClient{valid: false}
	})
	aispec.Register("test2", func() aispec.AIClient {
		test2 = true
		return &GetawayClient{valid: true}
	})
	client.SetGlobalNetworkConfig(context.Background(), config)

	// if not set ai type, use default config ai type
	ai.Chat("test")
	if !ok {
		t.Fatal("ai api priority failed")
	}

	// if not set ai type, use default config ai type and auto select valid ai type
	ok = false
	msg, err := ai.Chat("test")
	if !ok || !test1 || !test2 {
		t.Fatal("ai api priority failed")
	}
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "ok", msg)

	// is set ai type, but not registered, use default config ai type
	aispec.Register("ai", func() aispec.AIClient {
		return &GetawayClient{valid: false}
	})
	ok = false
	ai.Chat("test", aispec.WithType("ai"))
	if !ok {
		t.Fatal("ai api priority failed")
	}

	// is set ai type, and registered, and is valid
	ok = false
	var ok1 bool
	aispec.Register("ai", func() aispec.AIClient {
		ok1 = true
		return &GetawayClient{valid: true}
	})
	ai.Chat("test", aispec.WithType("ai"))
	if !(ok1 && !ok) {
		t.Fatal("ai api priority failed")
	}

	// is set ai type, and registered, and is invalid
	ok = false
	aispec.Register("ai", func() aispec.AIClient {
		ok1 = true
		return &GetawayClient{valid: false}
	})
	ai.Chat("test", aispec.WithType("ai"))
	if !(ok1 && ok) {
		t.Fatal("ai api priority failed")
	}

	functionCallOk := false
	times := 0
	aispec.Register("functionCall", func() aispec.AIClient {
		functionCallOk = true
		return &GetawayClient{valid: true, functionCallHandle: func(msg string) (map[string]any, error) {
			defer func() {
				times++
			}()
			if times != 3 {
				return nil, errors.New("error")
			} else {
				return map[string]any{
					"translate": "ok",
				}, nil
			}
		}}
	})
	res, err := ai.FunctionCall("test", map[string]any{"translate": "翻译为英文"}, aispec.WithType("functionCall"))
	if !functionCallOk {
		t.Fatal("ai api priority failed")
	}
	assert.Equal(t, "ok", res["translate"])
	assert.Equal(t, 4, times)
}

func TestGRPCMUSTPASS_COMMON_THIRDPARTY_APP(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}

	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		t.Fatal(err)
	}
	token := utils.RandSecret(100)
	config.AppConfigs = []*ypb.ThirdPartyApplicationConfig{
		{
			Type:   "github",
			APIKey: token,
		},
	}
	client.SetGlobalNetworkConfig(context.Background(), config)
	cfg := &aispec.AIConfig{}
	consts.GetThirdPartyApplicationConfig("github", cfg)
	if cfg.APIKey != token {
		t.Fatal("set thirdparty app config failed")
	}
}

func TestGRPCMUSTPASS_COMMON_GLOBAL_NETWORK_DNS_CONFIG(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		panic(err)
	}

	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})

	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		t.Fatal(err)
	}
	config.CustomDNSServers = []string{"127.0.0.1"}
	defer client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	check := false
	for _, i := range netx.NewDefaultReliableDNSConfig().SpecificDNSServers {
		if !check {
			if i == "127.0.0.1" {
				check = true
			}
		}
	}
	if !check {
		t.Fatal("set global network dns config failed")
	}
	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	check = false
	for _, i := range netx.NewDefaultReliableDNSConfig().SpecificDNSServers {
		if !check {
			if i == "127.0.0.1" {
				check = true
			}
		}
	}
	if check {
		t.Fatal("set (reset) global network dns config failed")
	}
}

func TestGRPCMUSTPASS_COMMON_RPOXY_FROM_ENV(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		panic(err)
	}

	triggerProxy := false
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		spew.Dump(req)
		if strings.Contains(string(req), "CONNECT 8.8.8.8:80") {
			triggerProxy = true
		}
		return nil
	})

	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}

	config.GlobalProxy = nil
	config.EnableSystemProxyFromEnv = true
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	defer func() {
		_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	}()
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("all_proxy")
	os.Unsetenv("proxy")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	os.Setenv("HTTP_PROXY", "http://"+utils.HostPort(host, port))
	_, err = yak.Execute(`
try {
	poc.Get("http://8.8.8.8")~
	die("unexpected result")
} catch e {
	if f"${e}".Contains("no proxy available") {
		dump(e)
		return
	} else{
		die(e)
	}
}
`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}

	if !triggerProxy {
		t.Fatal("proxy not triggered")
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
}

func TestGRPCMUSTPASS_COMMON_GLOBAL_RPOXY(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		panic(err)
	}

	triggerProxy := false
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		spew.Dump(req)
		if strings.Contains(string(req), "CONNECT 8.8.8.8:80") {
			triggerProxy = true
		}
		return nil
	})

	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}

	config.GlobalProxy = []string{"http://" + utils.HostPort(host, port)}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	defer func() {
		_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	}()
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("all_proxy")
	os.Unsetenv("proxy")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	_, err = yak.Execute(`
try {
	poc.Get("http://8.8.8.8")~
	die("unexpected result")
} catch e {
	if f"${e}".Contains("no proxy available") {
		dump(e)
		return
	} else{
		die(e)
	}
}
`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}

	if !triggerProxy {
		t.Fatal("proxy not triggered")
	}
}

func TestGRPCMUSTPASS_COMMON_DISALLOW_ADDRESS(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		panic(err)
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}

	config.DisallowIPAddress = []string{"8.8.8.8"}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	defer func() {
		_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	}()
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("all_proxy")
	os.Unsetenv("proxy")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	_, err = yak.Execute(`
try {
	poc.Get("http://8.8.8.8")~
	die("unexpected result")
} catch e {
	if f"${e}".Contains("disallow address") {
		dump(e)
		return
	} else{
		die(e)
	}
}
`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
}

func TestGRPCMUSTPASS_COMMON_DISALLOW_DOMAIN(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		panic(err)
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}

	config.DisallowDomain = []string{"a.com"}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	defer func() {
		_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	}()
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("all_proxy")
	os.Unsetenv("proxy")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	_, err = yak.Execute(`
try {
	poc.Get("http://a.com",poc.proxy("http://127.0.0.1:9999"))~
	die("unexpected result")
} catch e {
	if f"${e}".Contains("disallow domain") {
		dump(e)
		return
	} else{
		die(e)
	}
}
`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
}

func TestValidP12PassWord(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	ca, key, err := tlsutils.GenerateSelfSignedCertKey("127.0.0.1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, sKey, err := tlsutils.SignServerCrtNKeyEx(ca, key, "", false)
	if err != nil {
		t.Fatal(err)
	}
	p12Bytes, err := tlsutils.BuildP12(cert, sKey, "123456", ca)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ValidP12PassWord(context.Background(), &ypb.ValidP12PassWordRequest{Pkcs12Bytes: p12Bytes, Pkcs12Password: []byte("123456")})
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
}

func TestGRPCMUSTPASS_COMMON_HTTPAuth(t *testing.T) {
	username, passwd := "test", "test"
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		u, p, ok := request.BasicAuth()
		if ok && u == username && p == passwd {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		w.WriteHeader(http.StatusUnauthorized)
	})

	client, err := NewLocalClient(true)
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}
	target := fmt.Sprintf("%s:%d", host, port)
	config.AuthInfos = []*ypb.AuthInfo{{
		AuthType:     "any",
		AuthUsername: "test",
		AuthPassword: "test",
		Host:         target,
		Forbidden:    false,
	}, {
		AuthType:     "negotiate",
		AuthUsername: "test",
		AuthPassword: "test",
		Host:         target,
		Forbidden:    false,
	}, {
		AuthType:     "ntlm",
		AuthUsername: "test",
		AuthPassword: "testfasdf",
		Host:         target,
		Forbidden:    false,
	}}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}

	rsp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	_, statusCode, _ := lowhttp.GetHTTPPacketFirstLine(rsp.RawPacket)
	if statusCode != "200" {
		t.Fatalf("want 200 got %s", statusCode)
	}

	config.AuthInfos = []*ypb.AuthInfo{{
		AuthType:     "any",
		AuthUsername: "test",
		AuthPassword: "test",
		Host:         target,
		Forbidden:    true,
	}, {
		AuthType:     "negotiate",
		AuthUsername: "test",
		AuthPassword: "test",
		Host:         target,
		Forbidden:    false,
	}, {
		AuthType:     "ntlm",
		AuthUsername: "test",
		AuthPassword: "testfasdf",
		Host:         target,
		Forbidden:    false,
	}}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}

	rsp, err = lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	_, statusCode, _ = lowhttp.GetHTTPPacketFirstLine(rsp.RawPacket)
	if statusCode != "401" {
		t.Fatalf("want 401 got %s", statusCode)
	}

	config.AuthInfos = []*ypb.AuthInfo{{
		AuthType:     "any",
		AuthUsername: "test",
		AuthPassword: "test123",
		Host:         target,
		Forbidden:    false,
	}, {
		AuthType:     "basic",
		AuthUsername: "test",
		AuthPassword: "test",
		Host:         target,
		Forbidden:    false,
	}, {
		AuthType:     "ntlm",
		AuthUsername: "test",
		AuthPassword: "testfasdf",
		Host:         target,
		Forbidden:    false,
	}}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}

	rsp, err = lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	_, statusCode, _ = lowhttp.GetHTTPPacketFirstLine(rsp.RawPacket)
	if statusCode != "200" {
		t.Fatalf("want 200 got %s", statusCode)
	}
}

func TestPluginScanLists(t *testing.T) {
	client, err := NewLocalClient()
	require.Nil(t, err, "new local client error")

	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	host, port := utils.DebugMockHTTP([]byte("Hello"))
	recoverList := yakit.SetGlobalPluginScanLists([]string{}, []string{host})
	defer recoverList()

	manager, err := yak.NewMixPluginCaller()
	require.Nil(t, err, "new mix plugin caller error")

	token := utils.RandStringBytes(100)
	tmpName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	risk.NewRisk(%#v, risk.description("test"), risk.solution("test solution"))
}
`, token))
	require.Nil(t, err, "create temporary yak script error")
	defer clearFunc()
	manager.LoadPlugin(tmpName)
	t.Logf("GlobalPluginScanFilter lists: %#v", yakit.GlobalPluginScanFilter)
	manager.MirrorHTTPFlow(false, fmt.Sprintf("http://%s:%d", host, port), nil, nil, nil)
	_, ret, err := yakit.QueryRisks(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{Search: token})
	require.Nil(t, err, "query risks error")
	require.Len(t, ret, 0, "global network config plugin scan blacklist error")
}

//func TestHTTPAuth(t *testing.T) {
//	client, err := NewLocalClient()
//
//	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
//	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
//	if err != nil {
//		panic(err)
//	}

//		config.AuthInfos = []*ypb.AuthInfo{{
//			AuthType:     "any",
//			AuthUsername: "test",
//			AuthPassword: "test123",
//			Host:         "47.120.44.219:8087",
//		}, {
//			AuthType:     "negotiate",
//			AuthUsername: "test",
//			AuthPassword: "test",
//			Host:         "47.120.44.219:8087",
//		}, {
//			AuthType:     "ntlm",
//			AuthUsername: "test",
//			AuthPassword: "testfasdf",
//			Host:         "47.120.44.219:8087",
//		},
//		}
//		_, err = client.SetGlobalNetworkConfig(context.Background(), config)
//		if err != nil {
//			panic(err)
//		}
//
//		rsp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: 47.120.44.219:8087\r\n\r\n")))
//		if err != nil {
//			t.Fatal(err)
//		}
//		spew.Dump(rsp)
//	}
func TestGetThirdPartyAppConfigTemplate(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.GetThirdPartyAppConfigTemplate(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	apiTmp := res.GetTemplates()[0]
	for _, template := range apiTmp.GetItems() {
		fmt.Printf("name: %s, required: %v\n", template.GetName(), template.GetRequired())
	}
	assert.Equal(t, apiTmp.Name, aispec.RegisteredAIGateways()[0])
	assert.Equal(t, "api_key", apiTmp.Items[0].Name)
	assert.Equal(t, true, apiTmp.Items[0].Required)
	assert.Equal(t, "proxy", apiTmp.Items[4].Name)
	assert.Equal(t, "代理地址", apiTmp.Items[4].Verbose)

	assert.Equal(t, "model", apiTmp.Items[1].Name)
	assert.Equal(t, "模型名称", apiTmp.Items[1].Verbose)

	var comateTmp *ypb.GetThirdPartyAppConfigTemplate
	for _, t := range res.GetTemplates() {
		if t.GetName() == "comate" {
			comateTmp = t
			break
		}
	}
	assert.NotNil(t, comateTmp)
	assert.Equal(t, comateTmp.Items[0].GetRequired(), false)
}

func TestGenThirdPartyConfigOption(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.GetThirdPartyAppConfigTemplate(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	check := func(b bool) {
		if !b {
			t.Fatal("check failed")
		}
	}
	pass := make([]bool, 2)
	for _, tmp := range res.GetTemplates() {
		if tmp.Name == "openai" {
			pass[0] = true
			check(tmp.Items[0].Name == "api_key")
			check(tmp.Items[0].Verbose == "ApiKey")
			check(tmp.Items[1].Name == "model")
			check(tmp.Items[2].Name == "no_https")
			check(tmp.Items[3].Name == "domain")
			check(tmp.Items[4].Name == "proxy")
		}
		if tmp.Name == "fofa" {
			pass[1] = true
			check(tmp.Items[0].Name == "api_key")
			check(tmp.Items[1].Name == "user_identifier")
		}
	}
	if !funk.All(pass) {
		t.Fatal("check failed")
	}
}

func TestLoadThirdPartyConfig(t *testing.T) {
	config := yakit.GetNetworkConfig()
	bak := config.AppConfigs
	defer func() {
		config.AppConfigs = bak
		yakit.ConfigureNetWork(config)
	}()
	config.AppConfigs = nil
	yakit.ConfigureNetWork(config)

	config.AppConfigs = []*ypb.ThirdPartyApplicationConfig{
		{
			Type:           "openai",
			APIKey:         "APIKey",
			UserIdentifier: "UserIdentifier",
			UserSecret:     "UserSecret",
			Namespace:      "Namespace",
			Domain:         "Domain",
			WebhookURL:     "WebhookURL",
			ExtraParams: []*ypb.KVPair{
				{
					Key:   "no_https",
					Value: "true",
				},
				{
					Key:   "model",
					Value: "Model",
				},
				{
					Key:   "proxy",
					Value: "Proxy",
				},
			},
		},
	}
	yakit.ConfigureNetWork(config)
	aiConfig := &aispec.AIConfig{}
	err := consts.GetThirdPartyApplicationConfig("openai", aiConfig)
	if err != nil {
		t.Fatal(err)
	}
	spaceEngineConfig := &base.BaseSpaceEngineConfig{}
	err = consts.GetThirdPartyApplicationConfig("openai", spaceEngineConfig)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "APIKey", aiConfig.APIKey)
	assert.Equal(t, "Domain", aiConfig.Domain)
	assert.Equal(t, true, aiConfig.NoHttps)
	assert.Equal(t, "Model", aiConfig.Model)
	assert.Equal(t, "Proxy", aiConfig.Proxy)

	assert.Equal(t, "APIKey", spaceEngineConfig.APIKey)
	assert.Equal(t, "Domain", spaceEngineConfig.Domain)
	assert.Equal(t, "UserIdentifier", spaceEngineConfig.UserIdentifier)
}

func TestCallPluginTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()
	client, err := NewLocalClient(true)
	require.Nil(t, err, "new local client error")

	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	require.NoError(t, err)

	defaultBytes, _ := json.Marshal(config)
	defer yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(defaultBytes))

	config.CallPluginTimeout = 5
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	require.NoError(t, err)

	host, port := utils.DebugMockHTTP([]byte("Hello"))
	token := utils.RandStringBytes(20)
	code := fmt.Sprintf(`mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    time.sleep(2)
    yakit.StatusCard("%s", "%s")
}`, token, token)

	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code:       code,
		PluginType: "mitm",
		Input:      utils.HostPort(host, port),
	})
	require.NoError(t, err)

	checkOk := false
	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Warn(err)
		}
		if exec.IsMessage && strings.Contains(string(exec.Message), token) {
			checkOk = true
			break
		}
	}

	require.True(t, checkOk, "call plugin timeout failed")

	config.CallPluginTimeout = 1
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	require.NoError(t, err)

	stream, err = client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code:       code,
		PluginType: "mitm",
		Input:      utils.HostPort(host, port),
	})
	require.NoError(t, err)

	checkOk = false
	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Warn(err)
		}
		if exec.IsMessage && strings.Contains(string(exec.Message), token) {
			checkOk = true
			break
		}
	}
}

func TestInitNetworkConfig(t *testing.T) {
	emptyConfig := &ypb.GlobalNetworkConfig{}
	yakit.InitNetworkConfig(emptyConfig)
	defaultConfig := yakit.GetDefaultNetworkConfig()

	var notNeedCheckValue = []string{"ClientCertificates", "CustomDNSServers", "CustomDoHServers", "AuthInfos", "AiApiPriority"}

	defaultReflect := reflect.ValueOf(*defaultConfig)
	emptyReflect := reflect.ValueOf(*emptyConfig)
	for i := 0; i < defaultReflect.Type().NumField(); i++ {
		//get field name
		if defaultReflect.Type().Field(i).IsExported() {
			fieldName := defaultReflect.Type().Field(i).Name
			if !utils.StringArrayContains(notNeedCheckValue, fieldName) {
				require.Equal(t, defaultReflect.Field(i).Interface(), emptyReflect.Field(i).Interface(), fmt.Sprintf("init network config failed: fieldName[%s]", fieldName))
			}
		}

	}
}

package trafficguard

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

// ruleHit 判断 findings 中是否包含指定 ruleID 的命中。
func ruleHit(fs []Finding, ruleID int) bool {
	for _, f := range fs {
		if f.RuleID == ruleID {
			return true
		}
	}
	return false
}

// TestSuppressGoogleSelfHost 验证厂商自有域抑制: Google API Key 出现在 Google 自家域时不报,
// 出现在第三方域时仍报。对应实测误报: www.google.com / *.googleapis.com 携带的 AIza key。
func TestSuppressGoogleSelfHost(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	key := "AIzaSyDQ5Z4oX9pV2mN7bR3tK6cY1fH8eJ4gU5wX"
	req := []byte("GET /complete/search?sugkey=" + key + " HTTP/1.1\r\nHost: www.google.com\r\n\r\n")

	// Google 自有域: 抑制(自用, 非泄漏)。
	for _, host := range []string{"www.google.com", "content-autofill.googleapis.com", "fonts.gstatic.com"} {
		if fs := s.ScanHTTPFlow(host, req, nil); ruleHit(fs, 4) {
			t.Errorf("google key on self-host %q should be suppressed, got %v", host, summarize(fs))
		}
	}
	// 第三方域: 仍报(可能是真实泄漏)。
	if fs := s.ScanHTTPFlow("evil.example.com", req, nil); !ruleHit(fs, 4) {
		t.Errorf("google key on third-party host should be reported, got %v", summarize(fs))
	}
	// 无 host 上下文: 不做厂商域抑制, 仍报。
	if fs := s.ScanHTTPFlow("", req, nil); !ruleHit(fs, 4) {
		t.Errorf("google key without host context should be reported, got %v", summarize(fs))
	}
}

// TestSuppressGoogleFirstPartyApiKeyHeader 复刻实测误报: Chrome 自动填充请求
// content-autofill.googleapis.com 携带 x-goog-api-key: AIza...(同时命中规则 4/23/25)。
// 这类第一方自用流量应在 Google 自有域上被完全抑制; 同样的头出现在第三方域则仍应报。
func TestSuppressGoogleFirstPartyApiKeyHeader(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	// 真实形态: x-goog-api-key 头 + AIza key, 既命中 Google API Key(4), 又命中 api-key 字段(23)/头(25)。
	req := []byte("POST /v1/pages/ChVDaHJvbWUvMTQ HTTP/1.1\r\n" +
		"Host: content-autofill.googleapis.com\r\n" +
		"x-goog-api-key: AIzaSyDr2UxVnv_U8SAbhsY8XSHSIavtAW0DC-sY\r\n\r\n")

	// Google 自有域: 第一方自用, 全部抑制(规则 4/19/23/24/25 均不应命中)。
	for _, host := range []string{"content-autofill.googleapis.com", "www.google.com", "app-measurement.com"} {
		fs := s.ScanHTTPFlow(host, req, nil)
		for _, id := range []int{4, 19, 23, 24, 25} {
			if ruleHit(fs, id) {
				t.Errorf("first-party api-key noise on self-host %q should be suppressed, but rule %d hit: %v", host, id, summarize(fs))
			}
		}
	}
	// 第三方域: 同样的 api-key 头可能是真实泄漏, 仍应报(至少命中 Google API Key 规则 4)。
	if fs := s.ScanHTTPFlow("api.thirdparty.example", req, nil); !ruleHit(fs, 4) {
		t.Errorf("api-key on third-party host should still be reported, got %v", summarize(fs))
	}
}

// TestJWTDirectionAndAlg 验证 JWT 校验:
//   - 请求方向: 第一方会话凭证, 抑制(对应实测 data.bilibili.com 埋点请求里的 JWT 噪声);
//   - 响应方向 + 真实 JWT(首段含 alg): 保留(可能是硬编码/泄漏);
//   - 响应方向 + 非真 JWT(首段无 alg): 抑制(普通 base64 eyJ 块)。
func TestJWTDirectionAndAlg(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	realJWT := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	// 首段为 {"typ":"JWT","kid":"x"} (无 alg)的三段式 base64, 形似 JWT 但非真 JWT。
	noAlg := "eyJ0eXAiOiJKV1QiLCJraWQiOiJ4In0.eyJzdWIiOiIxMjM0NTY3ODkwIn0.AAAAAAAAAAAAAAAA"

	// 请求方向: 一律抑制(等同 Authorization 头第一方凭证)。
	if fs := s.ScanRequest([]byte("x-bili-ticket: " + realJWT)); ruleHit(fs, 19) {
		t.Errorf("JWT in request should be suppressed (first-party session), got %v", summarize(fs))
	}
	// 响应方向 + 真实 JWT: 保留。
	if fs := s.ScanResponse([]byte("var token = '" + realJWT + "';")); !ruleHit(fs, 19) {
		t.Errorf("real JWT hardcoded in response should be reported, got %v", summarize(fs))
	}
	// 响应方向 + 非真 JWT(无 alg): 抑制。
	if fs := s.ScanResponse([]byte("payload=" + noAlg)); ruleHit(fs, 19) {
		t.Errorf("non-JWT eyJ block (no alg) should be suppressed, got %v", summarize(fs))
	}
}

// TestSecretFieldTightening 验证规则23口令字段值收紧:
//   - 源码型值(JS 标识符/布尔/成员表达式/路径/方法调用/掩码/纯小写 slug): 抑制;
//   - 真实凭证型值(含数字/大小写混合/特殊字符): 保留(包括 JS 中硬编码的真凭证)。
func TestSecretFieldTightening(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	// 实测源码型误报样本(jquery / 百度 / B站 等 JS): 应抑制。
	falsePositives := []string{
		`password:true,image:true}`,
		`pwd="+encodeURIComponent(i.pwd))`,
		`password=x.auth.slice(p+1)`,
		`password=o.password,t.host=o.host`,
		`apiKey:e.data.get(`,
		`"api-key":"routes-api-failed"`,
		`passwd: '******'`,
		`Pwd:"/passApi/js/accSetPwd_2dc5c33.js"`,
		`secret:function(){return 1}`,
	}
	for _, fp := range falsePositives {
		if fs := s.ScanResponse([]byte(fp)); ruleHit(fs, 23) {
			t.Errorf("source-code-like secret field should be suppressed: %q -> %v", fp, summarize(fs))
		}
	}
	// 真实凭证型(包括 JS 硬编码): 应保留。
	truePositives := []string{
		`{"password":"S3cr3tP@ssw0rd2024"}`,
		`apiKey: "Ak9Lm2Xz7Qw8Rt5Yu3Vb1Nc"`,         // JS 硬编码长随机串
		`client_secret=GOCSPX-1a2B3c4D5e6F7g8H9iJ0kL`, // 含大小写+数字+前缀
	}
	for _, tp := range truePositives {
		if fs := s.ScanResponse([]byte(tp)); !ruleHit(fs, 23) {
			t.Errorf("real hardcoded secret should be reported: %q -> %v", tp, summarize(fs))
		}
	}
}

// TestLoginFormLabelNotReported 是用户反馈"访问任意登录页就报 password"误报的回归样本:
// 登录页/表单的本地化文案(i18n)、UI 占位/动作词被当成口令值是规则23最严重的误报源, 必须抑制。
// 这些样本均为"负样本"(登录框文案), 规则23一律不应命中。
func TestLoginFormLabelNotReported(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	negatives := []struct {
		name string
		data string
	}{
		// 复刻用户反馈截图: JS i18n 词典里的中文文案 password:"设置密码" / "忘记密码"。
		{"js-i18n-cjk", `var L={"zpglet.adjusted_post":"调整间距","password":"设置密码","zpglet.download_fjmd":"下载附件模板","name_already_exists":"名称已存在"};`},
		{"login-page-cjk", `{"login":"登录","username":"用户名","password":"请输入密码","confirmPassword":"确认密码"}`},
		{"forgot-pwd-cjk", `{"forgotPassword":"忘记密码","pwd":"忘记密码？"}`},
		{"reset-pwd-cjk", `{"password":"修改密码","newPassword":"新密码","oldPassword":"原密码"}`},
		// 英文 UI 文案: 值就是字段名 / 动作 / 占位词。
		{"en-label-word", `{"password":"Password","login":"Login","submit":"Submit","username":"Username"}`},
		{"en-placeholder-phrase", `{"password":"Enter your password","pwd":"Please input password"}`},
		// 压缩 JS 里的协议相对地址 //cdn 不应被误判成行注释而放行 CJK 文案。
		{"minified-protocol-relative-url", `var a="//cdn.example.com/lib.js";var L={"password":"设置密码"};`},
		// 页面里有一段"已闭合"的 HTML 注释, 之后脚本里的 CJK 文案仍应抑制(不在注释体内)。
		{"closed-html-comment-then-i18n", `<!-- page header --><script>var L={"password":"忘记密码"};</script>`},
	}
	for _, c := range negatives {
		t.Run(c.name, func(t *testing.T) {
			if fs := s.ScanResponse([]byte(c.data)); ruleHit(fs, 23) {
				t.Errorf("login-form label should NOT be reported as secret: %q -> %v", c.data, summarize(fs))
			}
		})
	}
}

// TestDefaultPasswordInCommentReported 验证"登录页/源码注释里写死的默认口令必须报出"。
// 与登录框文案相反, 被开发者写进注释的默认/初始口令是真实风险线索, 即便值看起来像普通词也要报。
// 这些样本均为"正样本"(注释中的默认口令), 规则23必须命中。
func TestDefaultPasswordInCommentReported(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	positives := []struct {
		name string
		data string
	}{
		// 强凭证型默认口令: 不在注释里也会报, 这里确认注释里同样报。
		{"html-comment-strong", `<!-- TODO 默认管理员 admin 默认密码 password: Admin@123456 -->`},
		// 弱口令但写在注释里: 常规上下文会被当 UI 词/slug 抑制, 注释里必须报。
		{"html-comment-weak-word", `<!-- 测试环境默认口令 password: adminpwd -->`},
		{"line-comment-slash", `// initial password: changeme (please reset after first login)`},
		{"block-comment", "/* default account\n   password: rootpass\n*/"},
		{"hash-comment-conf", "# db_password: changeme"},
		// 默认口令恰好等于字段名(常见占位式默认值)写在注释里也要报。
		{"comment-label-as-default", `<!-- password: password -->`},
	}
	for _, c := range positives {
		t.Run(c.name, func(t *testing.T) {
			if fs := s.ScanResponse([]byte(c.data)); !ruleHit(fs, 23) {
				t.Errorf("default password in comment MUST be reported: %q -> %v", c.data, summarize(fs))
			}
		})
	}
}

// TestRealSecretInLoginPageStillReported 验证收紧后真实凭证(即便出现在登录相关响应里)仍会报出,
// 避免"为了压误报把真泄漏也压没了"。
func TestRealSecretInLoginPageStillReported(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	positives := []string{
		`{"code":0,"data":{"username":"alice","password":"S3cr3tP@ssw0rd2024"}}`, // 接口返回明文真口令
		`{"password":"Xk9Lm2Zq7Rw8Tt5Yu3Vb"}`,                                     // 大小写+数字随机串
		`config: {"db_password":"Pg#2024Prod!"}`,                                  // 含特殊字符
	}
	for _, p := range positives {
		if fs := s.ScanResponse([]byte(p)); !ruleHit(fs, 23) {
			t.Errorf("real plaintext secret should still be reported: %q -> %v", p, summarize(fs))
		}
	}
}

// TestAuthHeaderRulesRemoved 验证 Authorization Bearer/Basic 规则(原20/21)已从内置规则集移除,
// 且 X-API-Key(25)、口令字段(23)、URL api_key(24) 仍在。
func TestAuthHeaderRulesRemoved(t *testing.T) {
	if _, ok := builtinRuleByID[20]; ok {
		t.Error("rule 20 (Authorization Bearer) should be removed")
	}
	if _, ok := builtinRuleByID[21]; ok {
		t.Error("rule 21 (Authorization Basic) should be removed")
	}
	for _, id := range []int{19, 23, 24, 25} {
		if _, ok := builtinRuleByID[id]; !ok {
			t.Errorf("rule %d should be retained", id)
		}
	}
	// 直接扫描带 Authorization 头的请求, 不应再因 Bearer/Basic 产生命中。
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	fs := s.ScanRequest([]byte("GET / HTTP/1.1\r\nAuthorization: Basic dXNlcjpwYXNzd29yZDEyMzQ=\r\n\r\n"))
	if ruleHit(fs, 20) || ruleHit(fs, 21) {
		t.Errorf("removed auth-header rules should not hit: %v", summarize(fs))
	}
}

// TestHighlightOffsetAlignment 验证命中偏移(From/To)与原始报文对齐: 据此 ExtractedData 的
// DataIndex/Length 才能在前端正确高亮。这里直接校验 buf[From:To] 即命中明文。
func TestHighlightOffsetAlignment(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	secret := "AKIAIOSFODNN7EXAMPLE"
	rsp := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nakid=" + secret + " end")
	fs := s.ScanResponse(rsp)
	var got *Finding
	for i := range fs {
		if fs[i].RuleID == 2 {
			got = &fs[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected AKIA finding, got %v", summarize(fs))
	}
	if got.From < 0 || got.To > len(rsp) || got.From >= got.To {
		t.Fatalf("invalid offsets From=%d To=%d len=%d", got.From, got.To, len(rsp))
	}
	if sub := string(rsp[got.From:got.To]); sub != secret {
		t.Errorf("offset misaligned: buf[From:To]=%q want %q", sub, secret)
	}
}

// TestAnnotateFlowPayload 验证命中流量会把"命中内容"写入 flow.Payload(供流量列表一眼可见)。
// trace 为空时不落库 extracted_data(无 DB 依赖), 仅校验 Payload。
func TestAnnotateFlowPayload(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	rsp := []byte("HTTP/1.1 200 OK\r\n\r\nakid=AKIAIOSFODNN7EXAMPLE")
	fs := s.ScanResponse(rsp)
	if len(fs) == 0 {
		t.Fatal("expected findings")
	}
	flow := &schema.HTTPFlow{Url: "https://x.example.com/a"} // 无 HiddenIndex -> 不写 extracted_data
	annotateFlowWithFindings(flow, fs, nil, rsp)
	if flow.Payload == "" {
		t.Error("flow.Payload should be populated with hit content")
	}
	if !strings.Contains(flow.Payload, "AKIA") {
		t.Errorf("payload should contain hit value, got %q", flow.Payload)
	}
}

// TestMergedRiskMarkdownContext 验证合并 Risk 的 markdown 描述给出命中值与前后上下文(便于判真假),
// 且严重度被压到中危(warning), Hash 基于 host/path + ruleID(降频去重)。
func TestMergedRiskMarkdownContext(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	rsp := []byte("HTTP/1.1 200 OK\r\n\r\nfoobar akid=AKIAIOSFODNN7EXAMPLE trailing")
	fs := s.ScanResponse(rsp)
	r := BuildMergedRisk(fs, "https://api.example.com/v1/data?ts=1", nil, rsp)
	if r == nil {
		t.Fatal("expected merged risk")
	}
	if r.Severity != "warning" {
		t.Errorf("severity should be capped to warning, got %q", r.Severity)
	}
	// 描述含命中值与上下文标注。
	if !strings.Contains(r.Description, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("description should contain the actual hit value for judgment:\n%s", r.Description)
	}
	if !strings.Contains(r.Description, "「") || !strings.Contains(r.Description, "」") {
		t.Errorf("description should mark hit in context with brackets:\n%s", r.Description)
	}

	// 降频去重: 同一 host/path、不同 query 的重复命中应得到相同 Hash。
	r2 := BuildMergedRisk(fs, "https://api.example.com/v1/data?ts=999", nil, rsp)
	if r2 == nil || r2.Hash != r.Hash {
		t.Errorf("same host/path + ruleIDs should produce identical Hash for dedup; got %q vs %q", r.Hash, mustHash(r2))
	}
	// 不同 path 应得到不同 Hash。
	r3 := BuildMergedRisk(fs, "https://api.example.com/v2/other", nil, rsp)
	if r3 != nil && r3.Hash == r.Hash {
		t.Errorf("different path should produce different Hash")
	}
}

func mustHash(r *schema.Risk) string {
	if r == nil {
		return "<nil>"
	}
	return r.Hash
}

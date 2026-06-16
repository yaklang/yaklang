package yakcmds

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func hotPatchTestContext(t *testing.T) {
	t.Helper()
	consts.GetGormProjectDatabase()
}

func mustBuildRequest(body string) []byte {
	raw := "POST /api/login HTTP/1.1\r\nHost: target.local\r\nContent-Type: text/plain\r\n\r\n" + body
	return lowhttp.FixHTTPRequest([]byte(raw))
}

func mustBuildResponse(status, body string) []byte {
	return []byte("HTTP/1.1 " + status + "\r\nContent-Type: text/plain\r\nContent-Length: " +
		utils.InterfaceToString(len(body)) + "\r\n\r\n" + body)
}

// 关键词: hotpatch-mitm, hijackHTTPRequest, 请求改写
func TestHotPatchMITM_HijackRequestModify(t *testing.T) {
	hotPatchTestContext(t)
	code := `
hijackHTTPRequest = func(isHttps, url, req, forward, drop) {
	if str.Contains(string(req), "/api/login") {
		forward(poc.ReplaceHTTPPacketBody(req, "modified=1"))
	}
}
`
	ctx, cancel := newHotPatchContext()
	defer cancel()
	res, err := RunMITMHotPatch(ctx, "mitm", code, false, "", mustBuildRequest("origin=1"), nil)
	require.NoError(t, err)
	require.True(t, res.RequestHooked, "request should be hooked")
	require.Contains(t, string(res.ModifiedRequest), "modified=1")
	require.False(t, res.Dropped)
}

// 关键词: hotpatch-mitm, hijackHTTPResponseEx, drop, 响应改写
func TestHotPatchMITM_StripJSAndDrop(t *testing.T) {
	hotPatchTestContext(t)

	rewriteCode := `
hijackHTTPResponseEx = func(isHttps, url, req, rsp, forward, drop) {
	body = string(poc.GetHTTPPacketBody(rsp))
	body = str.ReplaceAll(body, "alert(", "console.log(")
	forward(poc.ReplaceHTTPPacketBody(rsp, body))
}
`
	ctx, cancel := newHotPatchContext()
	defer cancel()
	res, err := RunMITMHotPatch(ctx, "mitm", rewriteCode, false, "",
		mustBuildRequest("x=1"), mustBuildResponse("200 OK", "alert('xss')"))
	require.NoError(t, err)
	require.True(t, res.ResponseHooked)
	require.Contains(t, string(res.ModifiedResponse), "console.log(")
	require.NotContains(t, string(res.ModifiedResponse), "alert(")

	dropCode := `
hijackHTTPRequest = func(isHttps, url, req, forward, drop) {
	if str.Contains(string(req), "/api/login") {
		drop()
	}
}
`
	res2, err := RunMITMHotPatch(ctx, "mitm", dropCode, false, "", mustBuildRequest("x=1"), nil)
	require.NoError(t, err)
	require.True(t, res2.Dropped)
	require.Equal(t, "request", res2.DropStage)
}

// 关键词: hotpatch-mitm, hijackSaveHTTPFlow, AddTag, 入库打标
func TestHotPatchMITM_SaveFlowTag(t *testing.T) {
	hotPatchTestContext(t)
	code := `
hijackSaveHTTPFlow = func(flow, modify, drop) {
	req = codec.StrconvUnquote(flow.Request)~
	if str.Contains(string(req), "password") {
		flow.AddTag("CREDENTIAL")
		flow.Red()
	}
	modify(flow)
}
`
	ctx, cancel := newHotPatchContext()
	defer cancel()
	res, err := RunMITMHotPatch(ctx, "mitm", code, false, "",
		mustBuildRequest("username=admin&password=123456"),
		mustBuildResponse("200 OK", "ok"))
	require.NoError(t, err)
	require.True(t, res.SaveHooked)
	require.Contains(t, res.SaveTags, "CREDENTIAL")
}

// 关键词: hotpatch-global, beforeRequest, afterRequest, 透明加解密
func TestHotPatchGlobal_TransparentCrypto(t *testing.T) {
	hotPatchTestContext(t)
	code := `
beforeRequest = func(isHttps, originReq, req) {
	body = poc.GetHTTPPacketBody(req)
	return poc.ReplaceHTTPPacketBody(req, codec.EncodeBase64(body))
}
afterRequest = func(isHttps, originReq, req, originRsp, rsp) {
	body = poc.GetHTTPPacketBody(rsp)
	dec = codec.DecodeBase64(body)~
	return poc.ReplaceHTTPPacketBody(rsp, dec)
}
`
	ctx, cancel := newHotPatchContext()
	defer cancel()
	// response body is base64("world") = d29ybGQ=
	res, err := RunMITMHotPatch(ctx, "global", code, false, "",
		mustBuildRequest("hello"), mustBuildResponse("200 OK", "d29ybGQ="))
	require.NoError(t, err)
	require.Contains(t, string(res.BeforeRequest), "aGVsbG8=", "request body should be base64 encoded")
	require.Contains(t, string(res.AfterResponse), "world", "response body should be base64 decoded")
}

// 关键词: hotpatch-webfuzzer, beforeRequest, afterRequest, customFailureChecker, retryHandler
func TestHotPatchWebFuzzer_BeforeAfterAndRetry(t *testing.T) {
	hotPatchTestContext(t)
	code := `
beforeRequest = func(https, originReq, req) {
	return poc.ReplaceHTTPPacketHeader(req, "X-Signed", "yes")
}
afterRequest = func(https, originReq, req, originRsp, rsp) {
	return poc.ReplaceHTTPPacketBody(rsp, "after-rewritten")
}
customFailureChecker = func(https, req, rsp, fail) {
	if str.Contains(string(rsp), "blocked") {
		fail("waf blocked keyword")
	}
}
retryHandler = func(https, retryCount, req, rsp, retryFunc) {
	if str.Contains(string(rsp), "405") {
		retryFunc(poc.ReplaceHTTPPacketMethod(req, "POST"))
	}
}
`
	ctx, cancel := newHotPatchContext()
	defer cancel()
	res, err := RunWebFuzzerHotPatch(ctx, code, false,
		mustBuildRequest("x=1"), mustBuildResponse("405 Method Not Allowed", "blocked by waf"), "")
	require.NoError(t, err)
	require.True(t, res.BeforeHooked)
	require.Contains(t, string(res.ModifiedRequest), "X-Signed")
	require.True(t, res.AfterHooked)
	require.Contains(t, string(res.ModifiedResponse), "after-rewritten")
	require.Len(t, res.FailureReasons, 1)
	require.Contains(t, res.FailureReasons[0], "waf blocked")
	require.Len(t, res.RetryRequests, 1)
	require.True(t, strings.HasPrefix(string(res.RetryRequests[0]), "POST "))
}

// 关键词: hotpatch-webfuzzer, fuzztag, {{yak(...)}} 渲染
func TestHotPatchWebFuzzer_Fuzztag(t *testing.T) {
	hotPatchTestContext(t)
	code := `
hashmd5 = func(s) {
	return codec.Md5(s)
}
`
	ctx, cancel := newHotPatchContext()
	defer cancel()
	res, err := RunWebFuzzerHotPatch(ctx, code, false, nil, nil, "{{yak(hashmd5|hello)}}")
	require.NoError(t, err)
	require.Len(t, res.RenderedFuzzTag, 1)
	require.Equal(t, "5d41402abc4b2a76b9719d911017c592", res.RenderedFuzzTag[0])
}

// 关键词: codec-plugin, handle, 右键 codec 调试
func TestRunCodecPlugin_Handle(t *testing.T) {
	hotPatchTestContext(t)
	code := `
handle = func(input) {
	return codec.EncodeBase64(input)
}
`
	ctx, cancel := newHotPatchContext()
	defer cancel()
	out, err := RunCodecPlugin(ctx, code, "hello")
	require.NoError(t, err)
	require.Equal(t, "aGVsbG8=", out)
}

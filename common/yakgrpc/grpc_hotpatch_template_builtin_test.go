package yakgrpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBuiltinHotPatchTemplateStartup(t *testing.T) {
	// Package initialization runs the same post-database hooks as the engine CLI.
	// Query through gRPC without manually installing the template.
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rsp, err := client.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{
		Name: []string{"HTTP流量测试项打标"}, Type: "global",
	})
	require.NoError(t, err)
	require.Len(t, rsp.Data, 1)
	require.Contains(t, rsp.Data[0].Content, "hijackSaveHTTPFlow")
	require.ElementsMatch(t, []string{"HTTP流量", "测试项打标"}, rsp.Data[0].Tags)
}

func TestBuiltinHotPatchTemplateLifecycle(t *testing.T) {
	s, err := newLiteYakServerForHotPatchTests(t)
	require.NoError(t, err)
	ctx := context.Background()
	const name = "HTTP流量测试项打标"
	filter := &ypb.HotPatchTemplateRequest{Name: []string{name}, Type: "global"}
	// A template of another type must not prevent the global default from appearing.
	_, err = s.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{Name: name, Type: "mitm", Content: "custom"})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		require.NoError(t, yakit.EnsureBuiltinHotPatchTemplates(s.GetProfileDatabase()))
	}
	rsp, err := s.QueryHotPatchTemplate(ctx, filter)
	require.NoError(t, err)
	require.Len(t, rsp.Data, 1)
	require.Contains(t, rsp.Data[0].Content, "hijackSaveHTTPFlow")
	require.ElementsMatch(t, []string{"HTTP流量", "测试项打标"}, rsp.Data[0].Tags)
	list, err := s.QueryHotPatchTemplateList(ctx, &ypb.QueryHotPatchTemplateListRequest{Type: "global"})
	require.NoError(t, err)
	require.Equal(t, []string{name}, list.Name)
	filtered, err := s.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{Type: "global", Tags: []string{"测试项打标"}})
	require.NoError(t, err)
	require.Len(t, filtered.Data, 1)
	tags, err := s.GetHotPatchTemplateTags(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.Contains(t, tags.Tags, &ypb.Tags{Value: "测试项打标", Total: 1})
	cfg, err := s.GetGlobalHotPatchConfig(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.Items)

	_, err = s.UpdateHotPatchTemplate(ctx, &ypb.UpdateHotPatchTemplateRequest{
		Condition: filter, Data: &ypb.HotPatchTemplate{Content: "// user edited", Tags: []string{"custom"}},
	})
	require.NoError(t, err)
	require.NoError(t, yakit.EnsureBuiltinHotPatchTemplates(s.GetProfileDatabase()))
	rsp, err = s.QueryHotPatchTemplate(ctx, filter)
	require.NoError(t, err)
	require.Len(t, rsp.Data, 1)
	require.Equal(t, "// user edited", rsp.Data[0].Content)
	require.Equal(t, []string{"custom"}, rsp.Data[0].Tags)
}

func TestBuiltinHotPatchTemplateFlowTags(t *testing.T) {
	s, err := newLiteYakServerForHotPatchTests(t)
	require.NoError(t, err)
	require.NoError(t, yakit.EnsureBuiltinHotPatchTemplates(s.GetProfileDatabase()))
	rsp, err := s.QueryHotPatchTemplate(context.Background(), &ypb.HotPatchTemplateRequest{Type: "global"})
	require.NoError(t, err)
	require.Len(t, rsp.Data, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	caller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	defer caller.Wait()
	require.NoError(t, caller.LoadHotPatch(ctx, nil, rsp.Data[0].Content))
	for _, tc := range []struct {
		name, method, path, headers, body, response string
		tags                                        []string
	}{
		{name: "plain", method: "GET", path: "/health"},
		{name: "injection and xss", method: "GET", path: "/search?q=hello", response: "Content-Type: text/html\r\n\r\n<html>hello</html>", tags: []string{"Injection", "SQLi", "XSS"}},
		{name: "access control", method: "POST", path: "/users/123", headers: "Cookie: session=demo\r\nContent-Type: application/json\r\n", body: `{"role":"admin"}`, tags: []string{"AccessControl", "IDOR", "MassAssignment", "Authentication", "CSRF"}},
		{name: "ssrf", method: "GET", path: "/fetch?url=http://localhost/private", tags: []string{"SSRF", "OpenRedirect"}},
		{name: "traversal", method: "GET", path: "/download?file=../../etc/passwd", tags: []string{"FileDownload", "PathTraversal", "FileInclusion"}},
		{name: "upload", method: "POST", path: "/upload", headers: "Content-Type: multipart/form-data; boundary=test\r\n", tags: []string{"FileUpload"}},
		{name: "command", method: "GET", path: "/run?cmd=whoami", tags: []string{"Injection", "SQLi", "CommandInjection"}},
		{name: "host parameter", method: "GET", path: "/ping?host=example.com", tags: []string{"CommandInjection"}},
		{name: "host form parameter", method: "POST", path: "/ping", headers: "Content-Type: application/x-www-form-urlencoded\r\n", body: "host=example.com", tags: []string{"CommandInjection"}},
		{name: "template", method: "GET", path: "/render?template={{7*7}}", tags: []string{"Injection", "SQLi", "SSTI"}},
		{name: "xml", method: "POST", path: "/soap", headers: "Content-Type: application/xml\r\n", body: "<root/>", tags: []string{"XXE"}},
		{name: "deserialization", method: "POST", path: "/data", body: `{"@type":"java.lang.Object"}`, tags: []string{"Deserialization"}},
		{name: "response exposure", method: "GET", path: "/info", headers: "Origin: https://example.com\r\n", response: "Access-Control-Allow-Origin: *\r\n\r\npassword=demo\nstack trace", tags: []string{"SensitiveInformation", "InformationDisclosure", "CORS"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := tc.method + " " + tc.path + " HTTP/1.1\r\nHost: example.com\r\n" + tc.headers + "\r\n" + tc.body
			response := "HTTP/1.1 200 OK\r\n" + tc.response
			flow := &schema.HTTPFlow{Url: "http://example.com" + tc.path, Tags: "existing"}
			flow.SetRequest(request)
			flow.SetResponse(response)
			var forwarded *schema.HTTPFlow
			dropped := false
			caller.HijackSaveHTTPFlow(flow, func(f *schema.HTTPFlow) {
				forwarded = f
			}, func() { dropped = true })
			caller.GetNativeCaller().Wait()
			require.Same(t, flow, forwarded, "save hook must explicitly forward the flow")
			require.False(t, dropped)
			expected := []string{"existing"}
			for _, tag := range tc.tags {
				expected = append(expected, "TEST:"+tag)
			}
			require.ElementsMatch(t, expected, strings.Split(flow.Tags, "|"))
			require.Equal(t, request, flow.GetRequest())
			require.Equal(t, response, flow.GetResponse())
		})
	}
}

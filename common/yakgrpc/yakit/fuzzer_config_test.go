package yakit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBuildWebFuzzerConfig(t *testing.T) {
	cfg, err := BuildWebFuzzerConfig(&ypb.FuzzerRequest{
		Request:      "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		IsHTTPS:      true,
		Concurrent:   30,
		HotPatchCode: "println(1)",
		ActualAddr:   "127.0.0.1:8080",
		Proxy:        "http://127.0.0.1:7890,http://127.0.0.1:7891",
	}, func(opts *WebFuzzerPageBuildOptions) {
		opts.TabName = "demo-tab"
		opts.PageID = "page-001"
	})
	require.NoError(t, err)
	require.Equal(t, "page-001", cfg.GetPageId())
	require.Equal(t, WebFuzzerConfigTypePage, cfg.GetType())

	var parsed WebFuzzerPageCacheItem
	require.NoError(t, json.Unmarshal([]byte(cfg.GetConfig()), &parsed))
	require.Equal(t, "page-001", parsed.ID)
	require.Equal(t, "0", parsed.GroupID)
	require.Equal(t, []any{}, parsed.GroupChildren)
	require.Equal(t, int64(1), parsed.SortField)
	require.Equal(t, "demo-tab", parsed.Verbose)
	require.True(t, parsed.PageParams.IsHttps)
	require.Equal(t, int64(30), parsed.PageParams.Concurrent)
	require.Equal(t, "println(1)", parsed.PageParams.HotPatchCode)
	require.Equal(t, "127.0.0.1:8080", parsed.PageParams.ActualHost)
	require.Equal(t, []string{"http://127.0.0.1:7890", "http://127.0.0.1:7891"}, parsed.PageParams.Proxy)
	require.Equal(t, defaultWebFuzzerParamItems(), parsed.PageParams.Params)
}

func TestBuildWebFuzzerConfig_RequireRequest(t *testing.T) {
	_, err := BuildWebFuzzerConfig(&ypb.FuzzerRequest{IsHTTPS: true})
	require.Error(t, err)
}

func TestBuildWebFuzzerConfig_OmitsUnsetMCPFields(t *testing.T) {
	cfg, err := BuildWebFuzzerConfig(&ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		IsHTTPS: false,
	}, func(opts *WebFuzzerPageBuildOptions) {
		opts.PageID = "page-002"
	})
	require.NoError(t, err)

	raw := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(cfg.GetConfig()), &raw))

	pageParams, ok := raw["pageParams"].(map[string]any)
	require.True(t, ok)
	_, hasConcurrent := pageParams["concurrent"]
	require.False(t, hasConcurrent)
	_, hasProxy := pageParams["proxy"]
	require.False(t, hasProxy)
	_, hasHotPatch := pageParams["hotPatchCode"]
	require.False(t, hasHotPatch)
	_, hasActualHost := pageParams["actualHost"]
	require.False(t, hasActualHost)
}

func TestBuildWebFuzzerConfig_RequestRawFallback(t *testing.T) {
	cfg, err := BuildWebFuzzerConfig(&ypb.FuzzerRequest{
		RequestRaw: []byte("GET /raw HTTP/1.1\r\nHost: example.com\r\n\r\n"),
		IsHTTPS:    true,
	})
	require.NoError(t, err)

	var parsed WebFuzzerPageCacheItem
	require.NoError(t, json.Unmarshal([]byte(cfg.GetConfig()), &parsed))
	require.Contains(t, parsed.PageParams.Request, "GET /raw")
}

func TestBuildWebFuzzerGroupConfig(t *testing.T) {
	config, err := BuildWebFuzzerGroupConfig("authentication", func(options *WebFuzzerGroupBuildOptions) {
		options.GroupID = "auth-group"
		options.SortField = 3
		options.Color = "purple"
		options.Expand = false
	})
	require.NoError(t, err)
	require.Equal(t, "auth-group", config.GetPageId())
	require.Equal(t, WebFuzzerConfigTypePageGroup, config.GetType())

	item, err := ParseWebFuzzerConfig(config)
	require.NoError(t, err)
	require.Equal(t, "auth-group", item.ID)
	require.Equal(t, "0", item.GroupID)
	require.Equal(t, "authentication", item.Verbose)
	require.Equal(t, int64(3), item.SortField)
	require.Equal(t, "purple", item.Color)
	require.False(t, item.Expand)
	require.Nil(t, item.PageParams)
}

func TestBuildWebFuzzerGroupConfigValidation(t *testing.T) {
	_, err := BuildWebFuzzerGroupConfig("  ")
	require.ErrorContains(t, err, "group name is required")

	_, err = BuildWebFuzzerGroupConfig(strings.Repeat("组", WebFuzzerGroupNameMaxLength+1))
	require.ErrorContains(t, err, "must not exceed 24 characters")

	_, err = BuildWebFuzzerGroupConfig("demo", func(options *WebFuzzerGroupBuildOptions) {
		options.GroupID = "not-a-yakit-group-id"
	})
	require.ErrorContains(t, err, "group id must end with group")

	_, err = BuildWebFuzzerGroupConfig("demo", func(options *WebFuzzerGroupBuildOptions) {
		options.Color = "pink"
	})
	require.ErrorContains(t, err, "unsupported group color")
}

func TestBuildWebFuzzerGroupConfigUsesDefaultColor(t *testing.T) {
	config, err := BuildWebFuzzerGroupConfig("baseline")
	require.NoError(t, err)

	item, err := ParseWebFuzzerConfig(config)
	require.NoError(t, err)
	require.Equal(t, WebFuzzerDefaultGroupColor, item.Color)
}

func TestBuildWebFuzzerGroupConfigAcceptsAndNormalizesCustomHexColor(t *testing.T) {
	config, err := BuildWebFuzzerGroupConfig("custom-color", func(options *WebFuzzerGroupBuildOptions) {
		options.Color = "#2f80ed"
	})
	require.NoError(t, err)

	item, err := ParseWebFuzzerConfig(config)
	require.NoError(t, err)
	require.Equal(t, "#2F80ED", item.Color)
}

func TestParseWebFuzzerConfigRejectsIdentityMismatch(t *testing.T) {
	config, err := BuildWebFuzzerConfig(&ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
	}, func(options *WebFuzzerPageBuildOptions) {
		options.PageID = "inside-id"
	})
	require.NoError(t, err)
	config.PageId = "outside-id"

	_, err = ParseWebFuzzerConfig(config)
	require.ErrorContains(t, err, "does not match")
}

func TestParseWebFuzzerProxy(t *testing.T) {
	require.Nil(t, parseWebFuzzerProxy(""))
	require.Nil(t, parseWebFuzzerProxy("  "))
	require.Equal(t, []string{"http://127.0.0.1:7890"}, parseWebFuzzerProxy("http://127.0.0.1:7890"))
	require.Equal(t, []string{"http://a", "http://b"}, parseWebFuzzerProxy("http://a, http://b"))
}

func TestWebFuzzerTabUpdatePushIsBackwardCompatible(t *testing.T) {
	raw, err := json.Marshal(&WebFuzzerTabPush{
		Action: WebFuzzerTabPushActionUpdate,
		ChangedData: []*ypb.FuzzerConfig{{
			PageId: "updated-page",
			Type:   WebFuzzerConfigTypePage,
		}},
	})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, WebFuzzerTabPushActionUpdate, payload["action"])
	require.NotContains(t, payload, "data", "legacy clients treat data as newly-created tabs")
	require.Contains(t, payload, "changedData")
}

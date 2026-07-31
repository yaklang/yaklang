package yakit

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestIsHTTPFlowBuiltinTag(t *testing.T) {
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagDiscarded))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagAutoFixResponse))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagResend))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagResend+"tag1"))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagWebFuzzer))

	require.False(t, IsHTTPFlowBuiltinTag("webfuzzer"))
	require.False(t, IsHTTPFlowBuiltinTag("custom-tag"))
}

func assertAllBuiltinTagsPresent(t *testing.T, tags []*TagAndStatusCode, expectCount map[string]int) {
	t.Helper()
	byValue := make(map[string]*TagAndStatusCode, len(tags))
	for _, tag := range tags {
		byValue[tag.Value] = tag
	}
	for builtin := range HTTPFlowBuiltinTags {
		got, ok := byValue[builtin]
		require.True(t, ok, "missing builtin tag %s", builtin)
		require.True(t, got.Builtin, "builtin tag %s should have Builtin=true", builtin)
		if expectCount != nil {
			if want, has := expectCount[builtin]; has {
				require.Equal(t, want, got.Count, "builtin tag %s count", builtin)
			}
		}
	}
}

func TestHTTPFlowTagsFromCounts(t *testing.T) {
	tags := HTTPFlowTagsFromCounts(map[string]int{
		"custom-tag":         3,
		HTTPFlowTagDiscarded: 2,
	})

	byValue := make(map[string]*TagAndStatusCode, len(tags))
	for _, tag := range tags {
		byValue[tag.Value] = tag
	}

	require.Equal(t, 3, byValue["custom-tag"].Count)
	require.False(t, byValue["custom-tag"].Builtin)

	require.Equal(t, 2, byValue[HTTPFlowTagDiscarded].Count)
	require.True(t, byValue[HTTPFlowTagDiscarded].Builtin)

	expect := make(map[string]int, len(HTTPFlowBuiltinTags))
	for builtin := range HTTPFlowBuiltinTags {
		expect[builtin] = 0
	}
	expect[HTTPFlowTagDiscarded] = 2
	assertAllBuiltinTagsPresent(t, tags, expect)
}

func TestHTTPFlowTagsFromCounts_EmptyAlwaysIncludesBuiltinZero(t *testing.T) {
	tags := HTTPFlowTagsFromCounts(nil)
	require.Len(t, tags, len(HTTPFlowBuiltinTags))

	expect := make(map[string]int, len(HTTPFlowBuiltinTags))
	for builtin := range HTTPFlowBuiltinTags {
		expect[builtin] = 0
	}
	assertAllBuiltinTagsPresent(t, tags, expect)
}

func TestQueryHTTPFlowTags_CountsAndBuiltin(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	customTag := "fieldgroup-custom-" + uuid.NewString()
	token := uuid.NewString()

	makeFlow := func(tags string) int64 {
		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithURL(fmt.Sprintf("http://example.com/%s", token)),
			CreateHTTPFlowWithTags(tags),
		)
		require.NoError(t, err)
		require.NoError(t, InsertHTTPFlow(db, flow))
		t.Cleanup(func() {
			_ = DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{Id: []int64{int64(flow.ID)}})
		})
		return int64(flow.ID)
	}

	makeFlow(customTag)
	makeFlow(customTag)
	makeFlow(HTTPFlowTagDiscarded)
	makeFlow(customTag + "|" + HTTPFlowTagDiscarded)

	tags, err := QueryHTTPFlowTags()
	require.NoError(t, err)

	byValue := make(map[string]*TagAndStatusCode, len(tags))
	for _, tag := range tags {
		byValue[tag.Value] = tag
	}

	require.Equal(t, 3, byValue[customTag].Count)
	require.False(t, byValue[customTag].Builtin)
	require.GreaterOrEqual(t, byValue[HTTPFlowTagDiscarded].Count, 2)
	require.True(t, byValue[HTTPFlowTagDiscarded].Builtin)

	assertAllBuiltinTagsPresent(t, tags, nil)
}

func TestHTTPFlowTags_CacheCountsAndBuiltin(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	customTag := "cache-fieldgroup-" + uuid.NewString()
	token := uuid.NewString()

	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL(fmt.Sprintf("http://example.com/%s", token)),
		CreateHTTPFlowWithTags(customTag+"|"+HTTPFlowTagWebFuzzer),
	)
	require.NoError(t, err)
	require.NoError(t, InsertHTTPFlow(db, flow))
	t.Cleanup(func() {
		_ = DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{Id: []int64{int64(flow.ID)}})
		model.DeleteHTTPFlowCacheGRPCModel(flow)
	})

	cached, err := model.ToHTTPFlowGRPCModel(flow, false)
	require.NoError(t, err)
	model.SetHTTPFlowCacheGRPCModel(flow, false, cached)

	tags, err := HTTPFlowTags(false)
	require.NoError(t, err)

	byValue := make(map[string]*TagAndStatusCode, len(tags))
	for _, tag := range tags {
		byValue[tag.Value] = tag
	}

	require.GreaterOrEqual(t, byValue[customTag].Count, 1)
	require.False(t, byValue[customTag].Builtin)
	require.GreaterOrEqual(t, byValue[HTTPFlowTagWebFuzzer].Count, 1)
	require.True(t, byValue[HTTPFlowTagWebFuzzer].Builtin)
	assertAllBuiltinTagsPresent(t, tags, nil)
}

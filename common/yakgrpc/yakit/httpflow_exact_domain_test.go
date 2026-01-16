package yakit

import (
	"fmt"
	"testing"

	"github.com/go-rod/rod/lib/utils"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHTTPFlowHostnameFilter(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := utils.RandString(10)
	var ids []int64

	testFlows := []*schema.HTTPFlow{
		{
			Url:     fmt.Sprintf("https://baidu.com/page1?t=%s", token),
			Host:    "baidu.com",
			Method:  "GET",
			IsHTTPS: true,
		},
		{
			Url:     fmt.Sprintf("https://foo.baidu.com/page2?t=%s", token),
			Host:    "foo.baidu.com",
			Method:  "GET",
			IsHTTPS: true,
		},
		{
			Url:     fmt.Sprintf("https://bar.baidu.com/page3?t=%s", token),
			Host:    "bar.baidu.com",
			Method:  "GET",
			IsHTTPS: true,
		},
		{
			Url:     fmt.Sprintf("https://google.com/page4?t=%s", token),
			Host:    "google.com",
			Method:  "GET",
			IsHTTPS: true,
		},
	}

	for _, flow := range testFlows {
		err := InsertHTTPFlow(db, flow)
		require.NoError(t, err)
		ids = append(ids, int64(flow.ID))
	}

	defer func() {
		if len(ids) > 0 {
			DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{Id: ids})
		}
	}()

	t.Run("HostnameFilter模糊匹配", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			HostnameFilter: []string{"baidu.com"},
			Keyword:        token,
			Full:           true,
		})
		require.NoError(t, err)
		require.Len(t, flows, 3)
	})

	t.Run("IncludeInUrl精确匹配", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			IncludeInUrl: []string{"foo.baidu.com"},
			Keyword:      token,
			Full:         true,
		})
		require.NoError(t, err)
		require.Len(t, flows, 1)
		require.Equal(t, "foo.baidu.com", flows[0].Host)
	})

	t.Run("交集查询-同源", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			HostnameFilter: []string{"baidu.com"},
			IncludeInUrl:   []string{"foo.baidu.com"},
			Keyword:        token,
			Full:           true,
		})
		require.NoError(t, err)
		require.Len(t, flows, 1)
		require.Equal(t, "foo.baidu.com", flows[0].Host)
	})

	t.Run("交集查询-异源为空", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			HostnameFilter: []string{"baidu.com"},
			IncludeInUrl:   []string{"google.com"},
			Keyword:        token,
			Full:           true,
		})
		require.NoError(t, err)
		require.Len(t, flows, 0)
	})

	t.Run("网站树多选", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			IncludeInUrl: []string{"foo.baidu.com", "bar.baidu.com"},
			Keyword:      token,
			Full:         true,
		})
		require.NoError(t, err)
		require.Len(t, flows, 2)
	})
}

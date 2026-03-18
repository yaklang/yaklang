package spacengine

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestShodanQuery(t *testing.T) {
	t.Skip("requires API key configured")
	res, err := ShodanQuery("*", "port:8080", 1, 10)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestFofaQuery(t *testing.T) {
	t.Skip("requires API key configured")
	res, err := FofaQuery("user", "pass", "domain=qq.com", 1, 30, 30)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestQuakeQuery(t *testing.T) {
	t.Skip("requires API key configured")
	res, err := QuakeQuery("", "service: http", 1, 30)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestHunterQuery(t *testing.T) {
	t.Skip("requires API key configured")
	res, err := HunterQuery("", `web.title="北京"`, 1, 10, 10)
	pass := false
	require.NoError(t, err)
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestZoomEyeQuery(t *testing.T) {
	t.Skip("requires API key configured")
	res, err := ZoomeyeQuery("", "site:baidu.com", 1, 10)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestZoneQuery(t *testing.T) {
	t.Skip("requires API key configured")
	res, err := ZoneQuery("", "(status_code=200)", 1, 10)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

// TestZoneQueryWithMockServer 使用 mock HTTP 服务测试 zone 查询，不依赖真实 API Key
func TestZoneQueryWithMockServer(t *testing.T) {
	mockData := `{"code":0,"data":[{"ip":"192.168.1.1","port":"80","url":"http://example.com","title":"Test Site","group":"TestCo","city":"Beijing"}]}`
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
			len(mockData), mockData))
	})
	mockDomain := fmt.Sprintf("http://%s:%d", host, port)

	ch, err := ZoneQueryWithConfig("fake-key", "(status_code=200)", 1, 10, nil, mockDomain)
	require.NoError(t, err)
	count := 0
	for r := range ch {
		count++
		require.Equal(t, "zone", r.FromEngine)
		require.Equal(t, "192.168.1.1:80", r.Addr)
		require.Equal(t, "Test Site", r.HtmlTitle)
		require.Contains(t, r.Location, "TestCo")
	}
	require.GreaterOrEqual(t, count, 1, "should get at least one result from mock")
}

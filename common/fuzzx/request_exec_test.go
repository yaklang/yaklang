package fuzzx

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestExec(t *testing.T) {
	key := "key"
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte(`HTTP/1.1 200 OK` + "\r\n\r\n" + lowhttp.GetHTTPRequestQueryParam(req, key))
	})
	iFuzztag := "{{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %s`, utils.HostPort(host, port)))
	freq := MustNewFuzzHTTPRequest(raw)
	ch, err := freq.FuzzGetParams(key, iFuzztag).Exec()
	require.NoError(t, err)
	gotMap := make(map[string]struct{})
	for r := range ch {
		gotMap[string(lowhttp.GetHTTPPacketBody(r.ResponseRaw))] = struct{}{}
	}

	require.Equal(t, len(excepts), len(gotMap))
	for _, e := range excepts {
		_, ok := gotMap[e]
		require.True(t, ok)
	}
}

func TestExecFirst(t *testing.T) {
	key := "key"
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte(`HTTP/1.1 200 OK` + "\r\n\r\n" + lowhttp.GetHTTPRequestQueryParam(req, key))
	})
	iFuzztag := "{{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %s`, utils.HostPort(host, port)))
	freq := MustNewFuzzHTTPRequest(raw)
	r, err := freq.FuzzGetParams(key, iFuzztag).ExecFirst()
	require.NoError(t, err)
	gotBody := string(lowhttp.GetHTTPPacketBody(r.ResponseRaw))
	require.Contains(t, excepts, gotBody)
}

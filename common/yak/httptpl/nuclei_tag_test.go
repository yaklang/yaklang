package httptpl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNucleiTag_RandStr(t *testing.T) {
	// res, err := ExecNucleiTag(`http://{{HostName}}:80/aaa`, map[string]any{
	// 	"HostName": "baidu.com",
	// })
}

func TestNucleiTag(t *testing.T) {
	// res, err := QuickFuzzNucleiTag(`http://{{HostName}}:80/aaa`, map[string]any{
	// 	"HostName": "baidu.com",
	// })
	// require.NoError(t, err)
	// require.Equal(t, "http://baidu.com:80/aaa", res)
	// 集束炸弹
	fuzzRes, err := FuzzNucleiTag("{{account}}:{{username}}-{{password}}", map[string]any{
		"account": "account",
	}, map[string][]string{
		"username": {"admin", "root"},
		"password": {"123456", "000000"},
	}, "")
	require.NoError(t, err)
	expect := []string{"account:admin-123456", "account:root-123456", "account:admin-000000", "account:root-000000"}
	for i, r := range fuzzRes {
		require.Equal(t, expect[i], string(r))
	}
	// 草叉模式
	fuzzRes, err = FuzzNucleiTag("{{account}}:{{username}}-{{password}}", map[string]any{
		"account": "account",
	}, map[string][]string{
		"username": {"admin", "root"},
		"password": {"123456", "000000"},
	}, "pitchfork")
	require.NoError(t, err)
	expect = []string{"account:admin-123456", "account:root-000000"}
	for i, r := range fuzzRes {
		require.Equal(t, expect[i], string(r))
	}
}

package yakit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// 确认新的实现不会再因为深度嵌套表达式导致查询报错，并能正常返回域名列表
func TestGetHTTPFlowDomainsByDomainSuffix_SimplifiedQuery(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	require.NotNil(t, db)

	token := uuid.NewString()

	records := []schema.HTTPFlow{
		{Url: "https://example.com/" + token + "/a/b", Path: "/a/b"},
		{Url: "https://example.com/" + token + "/c", Path: "/c"},
		{Url: "http://foo.com:8080/" + token, Path: "/"},
		// 一个较长的 URL，模拟可能导致表达式过深的输入
		{Url: "https://long.example.com/" + token + "/very/long/path/with/many/segments/for/testing", Path: "/very/long/path/with/many/segments/for/testing"},
	}

	// 插入测试数据
	for i := range records {
		require.NoError(t, db.Create(&records[i]).Error)
		defer db.Delete(&records[i])
	}

	// 仅过滤本次插入的记录
	query := db.Where("url LIKE ?", "%"+token+"%")
	res := GetHTTPFlowDomainsByDomainSuffix(query, "")
	require.NotNil(t, res)

	// 构建映射便于断言
	mp := map[string]*WebsiteNextPart{}
	for _, r := range res {
		mp[r.NextPart] = r
	}

	// 预期返回 example.com 和 foo.com:8080 两个域名
	require.Contains(t, mp, "https://example.com")
	require.Contains(t, mp, "http://foo.com:8080")

	// example.com 有两个记录，且有子路径
	require.Equal(t, 2, mp["https://example.com"].Count)
	require.True(t, mp["https://example.com"].HaveChildren)

	// foo.com:8080 只有一个记录，无子路径
	require.Equal(t, 1, mp["http://foo.com:8080"].Count)
}


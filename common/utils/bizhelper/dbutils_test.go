package bizhelper

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

type testData struct {
	gorm.Model
	Key   string
	Value int
}

func TestGroupColumn(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	db = db.Debug().AutoMigrate(&testData{}).Model(&testData{})

	token1, token2, token3 := utils.RandStringBytes(10), utils.RandStringBytes(10), utils.RandStringBytes(10)
	for i := 1; i < 6; i++ {
		db.Save(&testData{Key: token1, Value: i})
		db.Save(&testData{Key: token2, Value: i})
		db.Save(&testData{Key: token3, Value: i})
	}

	// test string
	data, err := GroupColumn(db, "test_data", "Key")
	require.NoError(t, err)
	require.Len(t, data, 3)
	for _, datum := range data {
		require.NotEmpty(t, datum)
	}

	fieldGroup := GroupCount(db, "test_data", "Key")
	require.Len(t, fieldGroup, 3)
	for _, group := range fieldGroup {
		require.Equal(t, int(group.Total), 5)
	}

	// test int
	data, err = GroupColumn(db, "test_data", "Value")
	require.NoError(t, err)
	require.Len(t, data, 5)
	for _, datum := range data {
		require.NotEmpty(t, datum)
	}

	fieldGroup = GroupCount(db, "test_data", "Value")
	require.Len(t, fieldGroup, 5)
	for _, group := range fieldGroup {
		require.Equal(t, int(group.Total), 3)
	}
}

// TestFuzzSearchNotEx 测试 FuzzSearchNotEx 函数，特别是特殊字符的转义处理
func TestFuzzSearchNotEx(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	type testSearchData struct {
		gorm.Model
		Content string
		Url     string
		Path    string
	}

	db = db.AutoMigrate(&testSearchData{}).Model(&testSearchData{})

	// 创建测试数据
	testCases := []struct {
		name    string
		content string
		url     string
		path    string
	}{
		{"normal", "normal content", "https://example.com/normal", "/normal"},
		{"with_percent", "content with 50% discount", "https://example.com/50%", "/50%"},
		{"with_underscore", "content_with_underscore", "https://example.com/file_name", "/file_name"},
		{"with_brackets", "content [test] value", "https://example.com/[test]", "/[test]"},
		{"with_caret", "content^test", "https://example.com/^test", "/^test"},
		{"with_backslash", "content\\test", "https://example.com/\\test", "/\\test"},
		{"mixed_special", "50%_discount[2024]^test\\value", "https://example.com/50%_discount[2024]^test\\value", "/50%_discount[2024]^test\\value"},
	}

	for _, tc := range testCases {
		db.Save(&testSearchData{
			Content: tc.content,
			Url:     tc.url,
			Path:    tc.path,
		})
	}

	t.Run("排除普通关键字", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content"}, "normal", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 应该排除包含 "normal" 的记录，剩下其他记录
		require.Greater(t, len(results), 0)
		for _, r := range results {
			require.NotContains(t, r.Content, "normal")
			require.NotContains(t, r.Url, "normal")
			require.NotContains(t, r.Path, "normal")
		}
	})

	t.Run("排除包含百分号的关键字", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content", "url", "path"}, "50%", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 应该排除包含 "50%" 的记录
		for _, r := range results {
			require.NotContains(t, r.Content, "50%")
			require.NotContains(t, r.Url, "50%")
			require.NotContains(t, r.Path, "50%")
		}
	})

	t.Run("排除包含下划线的关键字", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content", "url", "path"}, "with_underscore", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 应该排除包含 "with_underscore" 的记录
		for _, r := range results {
			require.NotContains(t, r.Content, "with_underscore")
			require.NotContains(t, r.Url, "with_underscore")
			require.NotContains(t, r.Path, "with_underscore")
		}
	})

	t.Run("排除包含方括号的关键字", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content", "url", "path"}, "[test]", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 应该排除包含 "[test]" 的记录
		for _, r := range results {
			require.NotContains(t, r.Content, "[test]")
			require.NotContains(t, r.Url, "[test]")
			require.NotContains(t, r.Path, "[test]")
		}
	})

	t.Run("排除包含脱字符的关键字", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content", "url", "path"}, "^test", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 应该排除包含 "^test" 的记录
		for _, r := range results {
			require.NotContains(t, r.Content, "^test")
			require.NotContains(t, r.Url, "^test")
			require.NotContains(t, r.Path, "^test")
		}
	})

	t.Run("排除包含反斜杠的关键字", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content", "url", "path"}, "\\test", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 应该排除包含 "\\test" 的记录
		for _, r := range results {
			require.NotContains(t, r.Content, "\\test")
			require.NotContains(t, r.Url, "\\test")
			require.NotContains(t, r.Path, "\\test")
		}
	})

	t.Run("排除包含多个特殊字符的关键字", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content", "url", "path"}, "50%_discount[2024]^test\\value", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 应该排除包含该混合特殊字符的记录
		for _, r := range results {
			require.NotContains(t, r.Content, "50%_discount[2024]^test\\value")
			require.NotContains(t, r.Url, "50%_discount[2024]^test\\value")
			require.NotContains(t, r.Path, "50%_discount[2024]^test\\value")
		}
	})

	t.Run("空关键字不应影响查询", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{"content"}, "", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 空关键字应该返回所有记录
		require.Greater(t, len(results), 0)
	})

	t.Run("空字段列表不应影响查询", func(t *testing.T) {
		result := FuzzSearchNotEx(db, []string{}, "test", false)
		var results []testSearchData
		err := result.Find(&results).Error
		require.NoError(t, err)
		// 空字段列表应该返回所有记录
		require.Greater(t, len(results), 0)
	})
}

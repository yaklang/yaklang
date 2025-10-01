package ziputil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewZipGrepSearcher(t *testing.T) {
	// 创建测试 ZIP
	files := map[string]string{
		"file1.txt":    "Line 1\nLine 2\nERROR: something wrong\nLine 4",
		"file2.txt":    "Content A\nContent B\nContent C",
		"logs/app.log": "INFO: started\nWARNING: slow\nERROR: failed\nDEBUG: details",
	}
	zipData := createTestZip(t, files)

	t.Run("create from raw data", func(t *testing.T) {
		searcher, err := NewZipGrepSearcherFromRaw(zipData)
		require.NoError(t, err)
		assert.NotNil(t, searcher)
		assert.Equal(t, 3, searcher.GetFileCount())
	})

	t.Run("create with filename", func(t *testing.T) {
		searcher, err := NewZipGrepSearcherFromRaw(zipData, "test.zip")
		require.NoError(t, err)
		assert.Contains(t, searcher.String(), "test.zip")
	})
}

func TestZipGrepSearcher_GrepRegexp(t *testing.T) {
	files := map[string]string{
		"file1.txt": "Line 1\nLine 2\nERROR: something wrong\nLine 4",
		"file2.txt": "Content A\nERROR: another error\nContent C",
		"file3.log": "INFO: info\nDEBUG: debug\nERROR: critical",
	}
	zipData := createTestZip(t, files)

	searcher, err := NewZipGrepSearcherFromRaw(zipData, "test.zip")
	require.NoError(t, err)

	t.Run("basic regexp search", func(t *testing.T) {
		results, err := searcher.GrepRegexp("ERROR:.*")
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// 应该找到 3 个 ERROR
		assert.Len(t, results, 3)

		for _, r := range results {
			assert.Contains(t, r.Line, "ERROR:")
		}
	})

	t.Run("regexp with context", func(t *testing.T) {
		results, err := searcher.GrepRegexp("ERROR:.*", WithContext(1))
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// 验证上下文
		for _, r := range results {
			if r.LineNumber > 1 {
				assert.NotEmpty(t, r.ContextBefore)
			}
		}
	})

	t.Run("regexp with limit", func(t *testing.T) {
		results, err := searcher.GrepRegexp("ERROR:.*", WithGrepLimit(2))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})
}

func TestZipGrepSearcher_GrepSubString(t *testing.T) {
	files := map[string]string{
		"file1.txt": "Hello World\nTest Content\nGoodbye",
		"file2.txt": "Another TEST file\nWith content",
	}
	zipData := createTestZip(t, files)

	searcher, err := NewZipGrepSearcherFromRaw(zipData)
	require.NoError(t, err)

	t.Run("case insensitive search", func(t *testing.T) {
		results, err := searcher.GrepSubString("test")
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// 应该找到 "Test" 和 "TEST"
		assert.Len(t, results, 2)
	})

	t.Run("case sensitive search", func(t *testing.T) {
		results, err := searcher.GrepSubString("Test", WithGrepCaseSensitive())
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Test Content", results[0].Line)
	})
}

func TestZipGrepSearcher_Cache(t *testing.T) {
	files := map[string]string{
		"file1.txt": "Content 1",
		"file2.txt": "Content 2",
		"file3.txt": "Content 3",
	}
	zipData := createTestZip(t, files)

	searcher, err := NewZipGrepSearcherFromRaw(zipData)
	require.NoError(t, err)

	t.Run("cache builds incrementally", func(t *testing.T) {
		// 初始缓存为空
		assert.Equal(t, 0, len(searcher.GetCachedFiles()))

		// 第一次搜索
		_, err := searcher.GrepSubString("Content")
		require.NoError(t, err)

		// 缓存应该包含所有访问的文件
		cached := searcher.GetCachedFiles()
		assert.Len(t, cached, 3)
	})

	t.Run("preload all files", func(t *testing.T) {
		searcher2, err := NewZipGrepSearcherFromRaw(zipData)
		require.NoError(t, err)

		// 预加载所有文件
		searcher2.WithCacheAll(true)

		// 缓存应该立即包含所有文件
		assert.Equal(t, 3, len(searcher2.GetCachedFiles()))
	})

	t.Run("clear cache", func(t *testing.T) {
		// 先搜索建立缓存
		_, err := searcher.GrepSubString("Content")
		require.NoError(t, err)
		assert.NotEmpty(t, searcher.GetCachedFiles())

		// 清空缓存
		searcher.ClearCache()
		assert.Empty(t, searcher.GetCachedFiles())
		assert.Equal(t, 0, searcher.GetCacheSize())
	})

	t.Run("get cache size", func(t *testing.T) {
		searcher3, err := NewZipGrepSearcherFromRaw(zipData)
		require.NoError(t, err)

		searcher3.WithCacheAll(true)
		size := searcher3.GetCacheSize()
		assert.Greater(t, size, 0)
		assert.Equal(t, len("Content 1")+len("Content 2")+len("Content 3"), size)
	})
}

func TestZipGrepSearcher_GrepInFile(t *testing.T) {
	files := map[string]string{
		"file1.txt": "Line 1\nLine 2\nERROR: in file1\nLine 4",
		"file2.txt": "Line 1\nERROR: in file2\nLine 3",
	}
	zipData := createTestZip(t, files)

	searcher, err := NewZipGrepSearcherFromRaw(zipData)
	require.NoError(t, err)

	t.Run("grep in specific file", func(t *testing.T) {
		results, err := searcher.GrepRegexpInFile("file1.txt", "ERROR:.*")
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "file1.txt", results[0].FileName)
		assert.Contains(t, results[0].Line, "file1")
	})

	t.Run("grep substring in specific file", func(t *testing.T) {
		results, err := searcher.GrepSubStringInFile("file2.txt", "ERROR")
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "file2.txt", results[0].FileName)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := searcher.GrepRegexpInFile("nonexistent.txt", "ERROR")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestZipGrepSearcher_GetFileContent(t *testing.T) {
	files := map[string]string{
		"test.txt": "Test Content\nLine 2\nLine 3",
	}
	zipData := createTestZip(t, files)

	searcher, err := NewZipGrepSearcherFromRaw(zipData)
	require.NoError(t, err)

	t.Run("get file content", func(t *testing.T) {
		content, err := searcher.GetFileContent("test.txt")
		require.NoError(t, err)
		assert.Equal(t, "Test Content\nLine 2\nLine 3", content)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := searcher.GetFileContent("nonexistent.txt")
		assert.Error(t, err)
	})
}

func TestZipGrepSearcher_Performance(t *testing.T) {
	// 创建大量文件
	files := make(map[string]string)
	for i := 0; i < 50; i++ {
		content := ""
		for j := 0; j < 100; j++ {
			if j%10 == 0 {
				content += "ERROR: error line\n"
			} else {
				content += "Normal line\n"
			}
		}
		files[string(rune('a'+i))+".txt"] = content
	}
	zipData := createTestZip(t, files)

	t.Run("search without cache vs with cache", func(t *testing.T) {
		// 不使用缓存
		searcher1, err := NewZipGrepSearcherFromRaw(zipData)
		require.NoError(t, err)

		// 第一次搜索
		results1, err := searcher1.GrepSubString("ERROR")
		require.NoError(t, err)
		assert.NotEmpty(t, results1)

		// 第二次搜索（应该使用缓存）
		results2, err := searcher1.GrepSubString("Normal")
		require.NoError(t, err)
		assert.NotEmpty(t, results2)

		// 验证缓存已建立
		assert.Equal(t, 50, len(searcher1.GetCachedFiles()))
	})

	t.Run("preload all for multiple searches", func(t *testing.T) {
		searcher2, err := NewZipGrepSearcherFromRaw(zipData)
		require.NoError(t, err)

		// 预加载
		searcher2.WithCacheAll(true)
		assert.Equal(t, 50, len(searcher2.GetCachedFiles()))

		// 多次搜索
		results1, err := searcher2.GrepSubString("ERROR")
		require.NoError(t, err)
		assert.NotEmpty(t, results1)

		results2, err := searcher2.GrepSubString("Normal")
		require.NoError(t, err)
		assert.NotEmpty(t, results2)

		results3, err := searcher2.GrepRegexp("ERROR:.*")
		require.NoError(t, err)
		assert.NotEmpty(t, results3)
	})
}

func TestZipGrepSearcher_String(t *testing.T) {
	files := map[string]string{
		"file1.txt": "Content 1",
		"file2.txt": "Content 2",
	}
	zipData := createTestZip(t, files)

	searcher, err := NewZipGrepSearcherFromRaw(zipData, "test.zip")
	require.NoError(t, err)

	str := searcher.String()
	assert.Contains(t, str, "test.zip")
	assert.Contains(t, str, "ZipGrepSearcher")
}

func TestZipGrepSearcher_ConcurrentAccess(t *testing.T) {
	files := map[string]string{
		"file1.txt": "Line 1\nERROR: test\nLine 3",
		"file2.txt": "Line 1\nWARNING: test\nLine 3",
		"file3.txt": "Line 1\nINFO: test\nLine 3",
	}
	zipData := createTestZip(t, files)

	searcher, err := NewZipGrepSearcherFromRaw(zipData)
	require.NoError(t, err)

	t.Run("concurrent searches", func(t *testing.T) {
		// 并发执行多个搜索
		done := make(chan bool, 3)

		go func() {
			_, err := searcher.GrepSubString("ERROR")
			assert.NoError(t, err)
			done <- true
		}()

		go func() {
			_, err := searcher.GrepSubString("WARNING")
			assert.NoError(t, err)
			done <- true
		}()

		go func() {
			_, err := searcher.GrepSubString("INFO")
			assert.NoError(t, err)
			done <- true
		}()

		// 等待所有搜索完成
		for i := 0; i < 3; i++ {
			<-done
		}

		// 验证缓存正常
		assert.Equal(t, 3, len(searcher.GetCachedFiles()))
	})
}

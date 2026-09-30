package yakgrpc

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// 历史问题：补全用的内置方法建议是进程级懒加载缓存，实现是
// `if len(m) == 0 { m[k] = v }` 这种无锁写法。并发补全请求（例如 IDE 同时
// 打开多个文件、或 grpc 并发调用）会同时进入初始化分支，导致
// `fatal error: concurrent map writes`，整个进程直接崩溃，无法 recover。
// 这里并发触发全部懒加载入口，保证初始化只发生一次且不 panic。
// 关键词: 并发补全初始化, concurrent map writes, sync.Once
func TestGRPCMUSTPASS_LANGUAGE_BuiltinSuggestionCacheConcurrentInit(t *testing.T) {
	getters := []func() any{
		func() any { return getLanguageKeywordSuggestions() },
		func() any { return getLanguageBasicTypeSuggestions() },
		func() any { return getStringBuiltinMethodSuggestions() },
		func() any { return getBytesBuiltinMethodSuggestions() },
		func() any { return getMapBuiltinMethodSuggestions() },
		func() any { return getSliceBuiltinMethodSuggestions() },
		func() any { return getStandardLibrarySuggestions() },
	}

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for _, get := range getters {
				require.NotEmpty(t, get())
			}
		}()
	}
	wg.Wait()

	// 缓存内容稳定：再次读取应与并发初始化结果一致。
	require.NotEmpty(t, getBytesBuiltinMethodSuggestions())
	require.Len(t, bytesBuiltinMethodSuggestionMap, len(bytesBuiltinMethod))
	require.Equal(t, len(bytesBuiltinMethodSuggestionMap), len(bytesBuiltinMethodSuggestions))
	require.Len(t, stringBuiltinMethodSuggestionMap, len(stringBuiltinMethod))
	require.Len(t, mapBuiltinMethodSuggestionMap, len(mapBuiltinMethod))
	require.Len(t, sliceBuiltinMethodSuggestionMap, len(sliceBuiltinMethod))
}

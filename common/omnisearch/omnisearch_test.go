package omnisearch

import (
	"fmt"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/omnisearch/mock"
	types "github.com/yaklang/yaklang/common/omnisearch/ostype"
	"gotest.tools/v3/assert"
)

// TestSearchByMock 测试使用Mock搜索引擎
// 验证API key为空时会返回错误，以及提供API key时的正确响应
func TestSearchByMock(t *testing.T) {
	osearch := NewOmniSearchClient(WithExtSearcher(mock.NewMockSearcher()))
	// 测试没有提供API key的情况
	_, err := osearch.Search("test123", types.WithSearchType(types.SearcherType("mock")), types.WithApiKey(""))
	assert.Error(t, err, "api key is required")

	// 测试提供有效API key的情况
	result, err := osearch.Search("test123", types.WithSearchType(types.SearcherType("mock")), types.WithApiKey("123"))
	assert.NilError(t, err)
	assert.Equal(t, result.Results[0].Content, "apikey: 123, mock test123")
}

// TestApiKeySwitching 测试API key自动切换功能
// 该测试验证在多次搜索请求中，系统能否根据hit count正确选择负载最小的API key
func TestApiKeySwitching(t *testing.T) {
	// 创建测试用的多个API keys
	testKeys := []*SearchKeyInfo{
		{
			ApiKey:   "key1",
			Type:     types.SearcherType("mock"),
			HitCount: 0,
		},
		{
			ApiKey:   "key2",
			Type:     types.SearcherType("mock"),
			HitCount: 0,
		},
		{
			ApiKey:   "key3",
			Type:     types.SearcherType("mock"),
			HitCount: 0,
		},
	}

	// 创建OmniSearch客户端并配置测试用的Mock搜索器和API keys
	mockSearcher := mock.NewMockSearcher()
	var osearchOptions []OmniSearchConfigOption
	for _, key := range testKeys {
		osearchOptions = append(osearchOptions, WithSearchKeys(key))
	}
	osearchOptions = append(osearchOptions, WithExtSearcher(mockSearcher))
	osearch := NewOmniSearchClient(osearchOptions...)

	// 第一次搜索应该使用key1（最低hit count）
	result1, err := osearch.Search("query1", types.WithSearchType(types.SearcherType("mock")))
	assert.NilError(t, err)
	assert.Equal(t, result1.Results[0].Content, "apikey: key1, mock query1")

	// 第二次搜索应该使用key2（此时key1的hit count为1）
	result2, err := osearch.Search("query2", types.WithSearchType(types.SearcherType("mock")))
	assert.NilError(t, err)
	assert.Equal(t, result2.Results[0].Content, "apikey: key2, mock query2")

	// 第三次搜索应该使用key3（此时key1和key2的hit count均为1）
	result3, err := osearch.Search("query3", types.WithSearchType(types.SearcherType("mock")))
	assert.NilError(t, err)
	assert.Equal(t, result3.Results[0].Content, "apikey: key3, mock query3")

	// 第四次搜索应该重新使用key1（此时所有key的hit count均为1）
	result4, err := osearch.Search("query4", types.WithSearchType(types.SearcherType("mock")))
	assert.NilError(t, err)
	assert.Equal(t, result4.Results[0].Content, "apikey: key1, mock query4")

	// 测试使用选项直接指定API key，覆盖自动选择逻辑
	resultOverride, err := osearch.Search("override",
		types.WithSearchType(types.SearcherType("mock")),
		types.WithApiKey("override-key"))
	assert.NilError(t, err)
	assert.Equal(t, resultOverride.Results[0].Content, "apikey: override-key, mock override")
}

// TestConcurrentApiKeySwitching 测试在并发环境下API key自动切换功能的正确性
// 该测试启动多个goroutine同时发起搜索请求，验证互斥锁的有效性和负载均衡的合理性
func TestConcurrentApiKeySwitching(t *testing.T) {
	// 创建测试用的API keys
	testKeys := []*SearchKeyInfo{
		{
			ApiKey:   "key1",
			Type:     types.SearcherType("mock"),
			HitCount: 0,
		},
		{
			ApiKey:   "key2",
			Type:     types.SearcherType("mock"),
			HitCount: 0,
		},
		{
			ApiKey:   "key3",
			Type:     types.SearcherType("mock"),
			HitCount: 0,
		},
	}

	// 创建搜索客户端
	mockSearcher := mock.NewMockSearcher()
	var osearchOptions []OmniSearchConfigOption
	for _, key := range testKeys {
		osearchOptions = append(osearchOptions, WithSearchKeys(key))
	}
	osearchOptions = append(osearchOptions, WithExtSearcher(mockSearcher))
	osearch := NewOmniSearchClient(osearchOptions...)

	// 并发测试参数设置
	concurrency := 10 // 并发goroutine数量
	iterations := 3   // 每个goroutine执行的搜索次数

	// 使用WaitGroup等待所有goroutine完成
	var wg sync.WaitGroup
	wg.Add(concurrency)

	// 启动并发goroutine执行搜索
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				query := fmt.Sprintf("query_goroutine_%d_iter_%d", id, j)
				_, err := osearch.Search(query, types.WithSearchType(types.SearcherType("mock")))
				assert.NilError(t, err)
			}
		}(i)
	}

	// 等待所有goroutine完成
	wg.Wait()

	// 验证所有key的hit count总和等于总搜索次数
	totalHitCount := 0
	for _, key := range testKeys {
		totalHitCount += key.HitCount
	}

	assert.Equal(t, totalHitCount, concurrency*iterations,
		"总hit count应等于总搜索次数")

	// 验证hit count分布是否相对均匀（没有单一key承担过多负载）
	maxDiff := 0
	minHitCount := testKeys[0].HitCount
	maxHitCount := testKeys[0].HitCount

	// 找出最大和最小的hit count
	for _, key := range testKeys {
		if key.HitCount < minHitCount {
			minHitCount = key.HitCount
		}
		if key.HitCount > maxHitCount {
			maxHitCount = key.HitCount
		}
	}

	// 计算最大差值
	maxDiff = maxHitCount - minHitCount

	// 在理想情况下，差值应该很小，但不一定为0
	// 在三个key的情况下，对于30次查询，最大差值不应超过5
	t.Logf("API Key使用分布情况: key1=%d, key2=%d, key3=%d, 最大差值=%d",
		testKeys[0].HitCount, testKeys[1].HitCount, testKeys[2].HitCount, maxDiff)

	// 断言最大差值在可接受范围内
	assert.Assert(t, maxDiff <= 5, "API Key使用分布不均匀: 最大差值=%d", maxDiff)
}

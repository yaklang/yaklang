package aid

import (
	"math/rand"
	runtimeLib "runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestEndpoint_Basic(t *testing.T) {
	t.Run("基本等待和激活测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		// 启动一个 goroutine 来等待
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			endpoint.Wait()
		}()

		// 短暂等待确保 goroutine 已经开始等待
		time.Sleep(100 * time.Millisecond)

		// 激活 endpoint
		params := aitool.InvokeParams{"test": "value"}
		manager.feed(endpoint.id, params)

		// 等待 goroutine 完成
		wg.Wait()

		// 验证参数是否正确传递
		receivedParams := endpoint.GetParams()
		assert.Equal(t, params, receivedParams)
	})

	t.Run("超时等待测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		// 测试超时情况
		timeout := 100 * time.Millisecond
		success := endpoint.WaitTimeout(timeout)
		assert.False(t, success, "应该超时返回")

		// 测试在超时前收到信号
		endpoint = manager.createEndpoint()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(50 * time.Millisecond)
			manager.feed(endpoint.id, aitool.InvokeParams{"test": "value"})
		}()

		success = endpoint.WaitTimeout(200 * time.Millisecond)
		assert.True(t, success, "应该在超时前收到信号")
		wg.Wait()
	})

	t.Run("多个等待者测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		const waiters = 5
		var wg sync.WaitGroup
		wg.Add(waiters)

		// 启动多个等待者
		for i := 0; i < waiters; i++ {
			go func() {
				defer wg.Done()
				endpoint.Wait()
			}()
		}

		// 确保所有 goroutine 都开始等待
		time.Sleep(100 * time.Millisecond)

		// 激活一次，应该唤醒所有等待者
		params := aitool.InvokeParams{"test": "value"}
		manager.feed(endpoint.id, params)

		// 等待所有 goroutine 完成
		wg.Wait()
	})

	t.Run("参数更新测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		// 测试参数更新
		params1 := aitool.InvokeParams{"key1": "value1"}
		params2 := aitool.InvokeParams{"key2": "value2"}

		manager.feed(endpoint.id, params1)
		receivedParams := endpoint.GetParams()
		assert.Equal(t, params1, receivedParams)

		manager.feed(endpoint.id, params2)
		receivedParams = endpoint.GetParams()
		assert.Equal(t, params2, receivedParams)
	})

	t.Run("并发安全测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		const goroutines = 10
		var wg sync.WaitGroup
		wg.Add(goroutines * 2) // 一半读取，一半写入

		// 启动多个 goroutine 并发读写
		for i := 0; i < goroutines; i++ {
			// 写入者
			go func(i int) {
				defer wg.Done()
				params := aitool.InvokeParams{
					"key": i,
				}
				manager.feed(endpoint.id, params)
			}(i)

			// 读取者
			go func() {
				defer wg.Done()
				endpoint.GetParams()
			}()
		}

		wg.Wait()
	})

	t.Run("无效 endpoint ID 测试", func(t *testing.T) {
		manager := newEndpointManager()

		// 测试使用不存在的 ID
		manager.feed("non-existent-id", aitool.InvokeParams{"test": "value"})

		// 测试使用空 ID
		manager.feed("", aitool.InvokeParams{"test": "value"})
	})

	t.Run("空参数测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		// 测试传入空参数
		manager.feed(endpoint.id, nil)
		params := endpoint.GetParams()
		assert.Empty(t, params)

		// 测试传入空 map
		manager.feed(endpoint.id, make(aitool.InvokeParams))
		params = endpoint.GetParams()
		assert.Empty(t, params)
	})

	t.Run("重复激活测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		// 连续多次激活同一个 endpoint
		for i := 0; i < 100; i++ {
			params := aitool.InvokeParams{"count": i}
			manager.feed(endpoint.id, params)

			// 验证最新的参数
			receivedParams := endpoint.GetParams()
			assert.Equal(t, params, receivedParams)
		}
	})

	t.Run("大量并发等待和激活测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		const (
			waiters  = 1000 // 等待者数量
			feeders  = 100  // 激活者数量
			duration = 2 * time.Second
		)

		var (
			wg            sync.WaitGroup
			activateCount int32
			waitCount     int32
		)

		// 启动等待者
		for i := 0; i < waiters; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for start := time.Now(); time.Since(start) < duration; {
					endpoint.Wait()
					atomic.AddInt32(&waitCount, 1)
				}
			}()
		}

		// 启动激活者
		for i := 0; i < feeders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for start := time.Now(); time.Since(start) < duration; {
					params := aitool.InvokeParams{
						"timestamp": time.Now().UnixNano(),
					}
					manager.feed(endpoint.id, params)
					atomic.AddInt32(&activateCount, 1)
					time.Sleep(time.Millisecond) // 给其他 goroutine 机会
				}
			}()
		}

		wg.Wait()
		t.Logf("总激活次数: %d, 总等待完成次数: %d", activateCount, waitCount)
		assert.True(t, waitCount > 0, "应该有等待被完成")
		assert.True(t, activateCount > 0, "应该有激活发生")
	})

	t.Run("参数竞争条件测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		const (
			goroutines = 100
			iterations = 1000
		)

		var wg sync.WaitGroup
		wg.Add(goroutines)

		// 创建一个通道来同步所有 goroutine 的开始
		start := make(chan struct{})

		for i := 0; i < goroutines; i++ {
			go func(id int) {
				defer wg.Done()

				// 等待开始信号
				<-start

				for j := 0; j < iterations; j++ {
					params := aitool.InvokeParams{
						"writer_id": id,
						"value":     j,
					}
					manager.feed(endpoint.id, params)

					// 立即读取参数
					received := endpoint.GetParams()

					// 验证读取的参数是有效的
					assert.NotNil(t, received)
					assert.NotNil(t, received["writer_id"])
					assert.NotNil(t, received["value"])
				}
			}(i)
		}

		// 发送开始信号
		close(start)
		wg.Wait()
	})

	t.Run("内存泄漏测试", func(t *testing.T) {
		if utils.InGithubActions() {
			t.Skip("skip memory leak test")
			return
		}

		if testing.Short() {
			t.Skip("跳过内存泄漏测试")
		}

		manager := newEndpointManager()
		const iterations = 10000

		var m1, m2 runtimeLib.MemStats
		runtimeLib.GC()
		runtimeLib.ReadMemStats(&m1)

		// 创建大量 endpoint 并激活它们
		for i := 0; i < iterations; i++ {
			endpoint := manager.createEndpoint()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				endpoint.Wait()
			}()
			manager.feed(endpoint.id, aitool.InvokeParams{"test": i})
			wg.Wait()
		}

		runtimeLib.GC()
		runtimeLib.ReadMemStats(&m2)

		// 检查内存增长是否在合理范围内
		memGrowth := m2.Alloc - m1.Alloc
		t.Logf("内存增长: %d bytes", memGrowth)
		// 假设每个 endpoint 不应该占用超过 1KB 内存
		assert.Less(t, memGrowth, uint64(iterations*1024),
			"内存使用增长超出预期")
	})

	t.Run("超长等待超时测试", func(t *testing.T) {
		if testing.Short() {
			t.Skip("跳过长时间测试")
		}

		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		// 测试较长的超时时间
		success := endpoint.WaitTimeout(5 * time.Second)
		assert.False(t, success, "长时间等待应该超时返回")

		// 测试在长时间等待期间的中断
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(2 * time.Second)
			manager.feed(endpoint.id, aitool.InvokeParams{"test": "value"})
		}()

		success = endpoint.WaitTimeout(10 * time.Second)
		assert.True(t, success, "应该在超时前收到信号")
		wg.Wait()
	})

	t.Run("随机延迟激活测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		const iterations = 50
		var wg sync.WaitGroup
		wg.Add(iterations)

		// 启动多个带有随机延迟的激活
		for i := 0; i < iterations; i++ {
			go func(i int) {
				defer wg.Done()
				// 随机延迟 0-100ms
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
				manager.feed(endpoint.id, aitool.InvokeParams{
					"iteration": i,
					"time":      time.Now().UnixNano(),
				})
			}(i)
		}

		// 等待所有激活完成
		wg.Wait()

		// 验证最后的参数
		finalParams := endpoint.GetParams()
		assert.NotNil(t, finalParams)
		assert.NotNil(t, finalParams["iteration"])
		assert.NotNil(t, finalParams["time"])
	})

	t.Run("参数深度拷贝测试", func(t *testing.T) {
		manager := newEndpointManager()
		endpoint := manager.createEndpoint()

		// 创建嵌套的参数结构
		nestedParams := aitool.InvokeParams{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": "value",
				},
			},
			"array": []interface{}{1, 2, 3},
		}

		manager.feed(endpoint.id, nestedParams)
		receivedParams := endpoint.GetParams()

		// 修改原始参数
		nestedMap := nestedParams["level1"].(map[string]interface{})
		nestedMap2 := nestedMap["level2"].(map[string]interface{})
		nestedMap2["level3"] = "modified"
		nestedParams["array"].([]interface{})[0] = 999

		// 验证接收到的参数没有被修改
		assert.Equal(t, "value",
			receivedParams["level1"].(map[string]interface{})["level2"].(map[string]interface{})["level3"])
		assert.Equal(t, 1, receivedParams["array"].([]interface{})[0])
	})
}

package aid

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

func TestEndpoint_Basic(t *testing.T) {
	t.Run("基本等待和激活测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

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
		manager.Feed(endpoint.GetId(), params)

		// 等待 goroutine 完成
		wg.Wait()

		// 验证参数是否正确传递
		receivedParams := endpoint.GetParams()
		assert.Equal(t, params, receivedParams)
	})

	t.Run("超时等待测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

		// 测试超时情况
		timeout := 100 * time.Millisecond
		success := endpoint.WaitTimeout(timeout)
		assert.False(t, success, "应该超时返回")

		// 测试在超时前收到信号
		endpoint = manager.CreateEndpoint()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(50 * time.Millisecond)
			manager.Feed(endpoint.GetId(), aitool.InvokeParams{"test": "value"})
		}()

		success = endpoint.WaitTimeout(200 * time.Millisecond)
		assert.True(t, success, "应该在超时前收到信号")
		wg.Wait()
	})

	//t.Run("多个等待者测试", func(t *testing.T) {
	//	manager := aicommon.NewEndpointManager()
	//	endpoint := manager.CreateEndpoint()
	//
	//	const waiters = 5
	//	var wg sync.WaitGroup
	//	wg.Add(waiters)
	//
	//	// 启动多个等待者
	//	for i := 0; i < waiters; i++ {
	//		go func() {
	//			defer wg.Done()
	//			endpoint.Wait()
	//		}()
	//	}
	//
	//	// 确保所有 goroutine 都开始等待
	//	time.Sleep(100 * time.Millisecond)
	//
	//	// 激活一次，应该唤醒所有等待者
	//	params := aitool.InvokeParams{"test": "value"}
	//	manager.Feed(endpoint.GetId(), params)
	//
	//	// 等待所有 goroutine 完成
	//	wg.Wait()
	//})

	t.Run("参数更新测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

		// 测试参数更新
		params1 := aitool.InvokeParams{"key1": "value1"}
		params2 := aitool.InvokeParams{"key2": "value2"}

		manager.Feed(endpoint.GetId(), params1)
		receivedParams := endpoint.GetParams()
		assert.Equal(t, params1, receivedParams)

		manager.Feed(endpoint.GetId(), params2)
		receivedParams = endpoint.GetParams()
		assert.Equal(t, params2, receivedParams)
	})

	t.Run("并发安全测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

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
				manager.Feed(endpoint.GetId(), params)
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
		manager := aicommon.NewEndpointManager()

		// 测试使用不存在的 ID
		manager.Feed("non-existent-id", aitool.InvokeParams{"test": "value"})

		// 测试使用空 ID
		manager.Feed("", aitool.InvokeParams{"test": "value"})
	})

	t.Run("空参数测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

		// 测试传入空参数
		manager.Feed(endpoint.GetId(), nil)
		params := endpoint.GetParams()
		assert.Empty(t, params)

		// 测试传入空 map
		manager.Feed(endpoint.GetId(), make(aitool.InvokeParams))
		params = endpoint.GetParams()
		assert.Empty(t, params)
	})

	t.Run("重复激活测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

		// 连续多次激活同一个 endpoint
		for i := 0; i < 100; i++ {
			params := aitool.InvokeParams{"count": i}
			manager.Feed(endpoint.GetId(), params)

			// 验证最新的参数
			receivedParams := endpoint.GetParams()
			assert.Equal(t, params, receivedParams)
		}
	})

	t.Run("参数竞争条件测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

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
					manager.Feed(endpoint.GetId(), params)

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

	t.Run("超长等待超时测试", func(t *testing.T) {
		if testing.Short() {
			t.Skip("跳过长时间测试")
		}

		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

		// 测试较短的超时时间
		success := endpoint.WaitTimeout(500 * time.Millisecond)
		assert.False(t, success, "等待应该超时返回")

		// 测试在等待期间的并发中断
		const goroutines = 10
		var wg sync.WaitGroup
		wg.Add(goroutines)

		// 创建通道同步所有 goroutine 的开始
		start := make(chan struct{})

		for i := 0; i < goroutines; i++ {
			go func(id int) {
				defer wg.Done()

				// 等待开始信号
				<-start

				// 随机延迟 50-250ms
				delay := 50 + rand.Intn(200)
				time.Sleep(time.Duration(delay) * time.Millisecond)

				manager.Feed(endpoint.GetId(), aitool.InvokeParams{
					"test": fmt.Sprintf("value-%d", id),
					"id":   id,
					"time": time.Now().UnixNano(),
				})

				log.Infof("goroutine %d fed endpoint after %dms", id, delay)
			}(i)
		}

		// 发送开始信号
		close(start)

		success = endpoint.WaitTimeout(1 * time.Second)
		assert.True(t, success, "应该在超时前收到信号")

		// 验证最后的参数
		finalParams := endpoint.GetParams()
		assert.NotNil(t, finalParams)
		assert.NotNil(t, finalParams["test"])
		assert.NotNil(t, finalParams["id"])
		assert.NotNil(t, finalParams["time"])

		wg.Wait()
	})

	t.Run("随机延迟激活测试", func(t *testing.T) {
		manager := aicommon.NewEndpointManager()
		endpoint := manager.CreateEndpoint()

		const iterations = 50
		var wg sync.WaitGroup
		wg.Add(iterations)

		// 启动多个带有随机延迟的激活
		for i := 0; i < iterations; i++ {
			go func(i int) {
				defer wg.Done()
				// 随机延迟 0-100ms
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
				manager.Feed(endpoint.GetId(), aitool.InvokeParams{
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
}

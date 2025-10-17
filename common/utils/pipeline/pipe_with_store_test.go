package pipeline_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pipeline"
)

func TestPipeWithStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var workerCount atomic.Int32

	// 初始化函数：为每个 worker 创建本地存储
	initWorker := func() *utils.SafeMap[any] {
		store := utils.NewSafeMap[any]()
		workerID := workerCount.Add(1)
		store.Set("worker_id", workerID)
		store.Set("processed_count", 0)
		log.Infof("Worker %d initialized", workerID)
		return store
	}

	// 处理函数：使用 store 来跟踪每个 worker 处理的数据
	handler := func(item int, store *utils.SafeMap[any]) (string, error) {
		workerID, _ := store.Get("worker_id")
		countVal, _ := store.Get("processed_count")
		count := countVal.(int)
		count++
		store.Set("processed_count", count)

		log.Infof("Worker %v processing item %d (total processed: %d)", workerID, item, count)

		if item%2 != 0 {
			return "", fmt.Errorf("odd number")
		}

		return fmt.Sprintf("w%v-i%d-c%d", workerID, item*2, count), nil
	}

	// 使用 3 个 worker 并发处理
	p := pipeline.NewPipeWithStore(ctx, 10, handler, initWorker, 3)

	go func() {
		for i := 0; i < 20; i++ {
			p.Feed(i)
		}
		p.Close()
	}()

	var results []string
	for result := range p.Out() {
		results = append(results, result)
	}

	// 验证结果数量（只有偶数）
	assert.Equal(t, 10, len(results))

	// 验证所有结果都包含 worker ID 和处理计数
	for _, result := range results {
		assert.Contains(t, result, "w")
		assert.Contains(t, result, "-i")
		assert.Contains(t, result, "-c")
		log.Infof("Result: %s", result)
	}

	// 验证创建了 3 个 worker
	assert.Equal(t, int32(3), workerCount.Load())
}

func TestPipeWithStoreCounter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 简单的计数器示例
	initWorker := func() *utils.SafeMap[any] {
		store := utils.NewSafeMap[any]()
		store.Set("counter", 0)
		return store
	}

	handler := func(item int, store *utils.SafeMap[any]) (int, error) {
		counterVal, _ := store.Get("counter")
		counter := counterVal.(int)
		counter++
		store.Set("counter", counter)
		return item + counter, nil
	}

	p := pipeline.NewPipeWithStore(ctx, 10, handler, initWorker, 2)

	// 输入数据
	input := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p.FeedSlice(input)

	var results []int
	for result := range p.Out() {
		results = append(results, result)
	}

	// 每个结果应该是原始值 + 该 worker 的处理计数
	assert.Equal(t, 10, len(results))

	// 所有结果都应该大于原始输入
	for _, result := range results {
		assert.Greater(t, result, 0)
		log.Infof("Result: %d", result)
	}
}

func TestPipeWithStoreChained(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Pipe 1: 带 store 的处理，添加 worker 标识
	initWorker1 := func() *utils.SafeMap[any] {
		store := utils.NewSafeMap[any]()
		store.Set("prefix", "p1")
		return store
	}

	handler1 := func(item int, store *utils.SafeMap[any]) (string, error) {
		prefixVal, _ := store.Get("prefix")
		prefix := prefixVal.(string)
		return fmt.Sprintf("%s-%d", prefix, item+1), nil
	}

	pipe1 := pipeline.NewPipeWithStore(ctx, 10, handler1, initWorker1, 2)

	// Pipe 2: 普通处理（无 store）
	handler2 := func(item string) (string, error) {
		return item + "-processed", nil
	}

	pipe2 := pipeline.NewPipe(ctx, 10, handler2, 2)

	// 启动处理链
	pipe1.FeedSlice([]int{0, 1, 2, 3, 4})
	pipe2.FeedChannel(pipe1.Out())

	var results []string
	for result := range pipe2.Out() {
		results = append(results, result)
		log.Infof("Final result: %s", result)
	}

	assert.Equal(t, 5, len(results))

	// 验证所有结果都包含预期的格式
	for _, result := range results {
		assert.Contains(t, result, "p1-")
		assert.Contains(t, result, "-processed")
	}
}

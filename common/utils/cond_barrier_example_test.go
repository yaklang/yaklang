package utils

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// ExampleCondBarrier_BasicUsage 演示基本的条件屏障使用方法
func ExampleCondBarrier_BasicUsage() {
	cb := NewCondBarrier()

	// 启动三个并发任务
	go func() {
		cond1 := cb.CreateBarrier("condition1")
		defer cond1.Done()
		time.Sleep(100 * time.Millisecond)
		log.Info("condition1 completed")
	}()

	go func() {
		cond2 := cb.CreateBarrier("condition2")
		defer cond2.Done()
		time.Sleep(200 * time.Millisecond)
		log.Info("condition2 completed")
	}()

	go func() {
		cond3 := cb.CreateBarrier("condition3")
		defer cond3.Done()
		time.Sleep(150 * time.Millisecond)
		log.Info("condition3 completed")
	}()

	// 分层等待
	cb.Wait("condition1")
	log.Info("condition1 is done, continuing...")

	cb.Wait("condition2")
	log.Info("condition2 is done, continuing...")

	cb.WaitAll()
	log.Info("all conditions done!")
}

// ExampleCondBarrier_Reentrant 演示重入功能（类似 WaitGroup）
func ExampleCondBarrier_Reentrant() {
	cb := NewCondBarrier()
	var counter int
	var mu sync.Mutex

	// 创建多个相同名称的屏障（重入功能）
	const numWorkers = 5
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			barrier := cb.CreateBarrier("workers")
			defer barrier.Done()

			// 模拟工作
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			counter++
			mu.Unlock()

			log.Infof("Worker %d finished", id)
		}(i)
	}

	// 等待所有工作者完成
	cb.Wait("workers")

	mu.Lock()
	fmt.Printf("All %d workers completed\n", counter)
	mu.Unlock()

	// Output: All 5 workers completed
}

// ExampleCondBarrier_AddMethod 演示 Add 方法的使用
func ExampleCondBarrier_AddMethod() {
	cb := NewCondBarrier()

	// 创建一个屏障并增加计数
	barrier := cb.CreateBarrier("batch_job")
	barrier.Add(2) // 总共需要3次Done调用（1个初始 + 2个Add）

	// 启动3个任务
	for i := 0; i < 3; i++ {
		go func(id int) {
			defer barrier.Done()
			time.Sleep(50 * time.Millisecond)
			log.Infof("Task %d completed", id)
		}(i)
	}

	// 等待所有任务完成
	cb.Wait("batch_job")
	log.Info("All batch jobs completed")
}

// ExampleCondBarrier_MultipleWait 演示等待多个条件
func ExampleCondBarrier_MultipleWait() {
	cb := NewCondBarrier()

	// 启动多个任务
	tasks := []string{"database", "cache", "api", "auth"}
	for i, task := range tasks {
		go func(taskName string, delay int) {
			barrier := cb.CreateBarrier(taskName)
			defer barrier.Done()
			time.Sleep(time.Duration(delay) * time.Millisecond)
			log.Infof("%s service ready", taskName)
		}(task, (i+1)*25)
	}

	// 等待核心服务（数据库和缓存）准备完成
	cb.Wait("database", "cache")
	log.Info("Core services are ready, starting application...")

	// 等待所有服务
	cb.WaitAll()
	log.Info("All services are ready!")
}

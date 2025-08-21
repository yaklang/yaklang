package yaklib

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type WaitGroupProxy struct {
	sync.WaitGroup

	ctx   context.Context
	count atomic.Int64
}

// SetContext sets the context for the WaitGroup.
// ! If Call twice or more, any of the previous context Done will cause the WaitGroup to be SetZero.
func (wg *WaitGroupProxy) SetContext(ctx context.Context) {
	wg.ctx = ctx
	go func() {
		<-ctx.Done()
		wg.SetZero()
	}()
}

func (wg *WaitGroupProxy) Add(deltas ...int) {
	delta := 1
	if len(deltas) > 0 {
		delta = deltas[0]
	}
	if delta < 0 && wg.count.Load()+int64(delta) < 0 {
		delta = 0 - int(wg.count.Load())
	}
	wg.count.Add(int64(delta))
	wg.WaitGroup.Add(delta)
}

func (wg *WaitGroupProxy) SetZero() {
	wg.Add(0 - int(wg.count.Load()))
}

func (wg *WaitGroupProxy) Done() {
	defer func() {
		if r := recover(); r != nil {
			if errMsg := utils.InterfaceToString(r); errMsg == "sync: negative WaitGroup counter" {
				log.Error(errMsg)
			} else {
				panic(r)
			}
		}
	}()
	wg.Add(-1)
}

// NewWaitGroup 创建一个 WaitGroup 结构体引用，其帮助我们在处理多个并发任务时，等待所有任务完成后再进行下一步操作
// Example:
// ```
// wg = sync.NewWaitGroup()
// for i in 5 {
// wg.Add() // 增加一个任务
// go func(i) {
// defer wg.Done()
// time.Sleep(i)
// printf("任务%d 完成\n", i)
// }(i)
// }
// wg.Wait()
// println("所有任务完成")
// ```
func NewWaitGroup(ctxs ...context.Context) *WaitGroupProxy {
	wg := &WaitGroupProxy{}
	for _, ctx := range ctxs {
		wg.SetContext(ctx)
	}
	return wg
}

// NewSizedWaitGroup 创建一个 SizedWaitGroup 结构体引用，其帮助我们在处理多个并发任务时，等待所有任务完成后再进行下一步操作
// SizedWaitGroup 与 WaitGroup 的区别在于 SizedWaitGroup 可以限制并发任务的数量
// Example:
// ```
// wg = sync.NewSizedWaitGroup(5) // 限制大小为5
// for i in 10 {
// wg.Add() // 任务数量超过5时，会阻塞，直到有任务完成
// go func(i) {
// defer wg.Done()
// time.Sleep(i)
// printf("任务%d 完成\n", i)
// }(i)
// }
// wg.Wait()
// println("所有任务完成")
// ```
func NewSizedWaitGroup(size int, ctxs ...context.Context) *utils.SizedWaitGroup {
	return utils.NewSizedWaitGroup(size, ctxs...)
}

// NewMutex 创建一个 Mutex 结构体引用，用于实现互斥锁，其帮助我们避免多个并发任务访问同一个资源时出现数据竞争问题
// Example:
// ```
// m = sync.NewMutex()
// newMap = make(map[string]string)
// go func{
// for {
// m.Lock()         // 请求锁
// defer m.Unlock() // 释放锁
// newMap["key"] = "value" // 防止多个并发任务同时修改 newMap
// }
// }
// for {
// println(newMap["key"])
// }
// ```
func NewMutex() *sync.Mutex {
	return new(sync.Mutex)
}

// NewRWMutex 创建一个 RWMutex 结构体引用，用于实现读写锁，其帮助我们避免多个并发任务访问同一个资源时出现数据竞争问题
// Example:
// ```
// m = sync.NewRWMutex()
// newMap = make(map[string]string)
// go func{
// for {
// m.Lock()         // 请求写锁
// defer m.Unlock() // 释放写锁
// newMap["key"] = "value" // 防止多个并发任务同时修改 newMap
// }
// }
// for {
// m.RLock()         // 请求读锁
// defer m.RUnlock() // 释放读锁
// println(newMap["key"])
// }
// ```
func NewRWMutex() *sync.RWMutex {
	return new(sync.RWMutex)
}

// NewLock 创建一个 Mutex 结构体引用，用于实现互斥锁，其帮助我们避免多个并发任务访问同一个资源时出现数据竞争问题
// 它实际是 NewMutex 的别名
// Example:
// ```
// m = sync.NewMutex()
// newMap = make(map[string]string)
// go func{
// for {
// m.Lock()         // 请求锁
// defer m.Unlock() // 释放锁
// newMap["key"] = "value" // 防止多个并发任务同时修改 newMap
// }
// }
// for {
// println(newMap["key"])
// }
// ```
func NewLock() *sync.Mutex {
	return new(sync.Mutex)
}

// NewMap 创建一个 Map 结构体引用，这个 Map 是并发安全的
// Example:
// ```
// m = sync.NewMap()
// go func {
// for {
// m.Store("key", "value2")
// }
// }
// for {
// m.Store("key", "value")
// v, ok = m.Load("key")
// if ok {
// println(v)
// }
// }
// ```
func NewMap() *sync.Map {
	return new(sync.Map)
}

// NewOnce 创建一个 Once 结构体引用，其帮助我们确保某个函数只会被执行一次
// Example:
// ```
// o = sync.NewOnce()
// for i in 10 {
// o.Do(func() { println("this message will only print once") })
// }
// ```
func NewOnce() *sync.Once {
	return new(sync.Once)
}

// NewPool 创建一个 Pool 结构体引用，其帮助我们复用临时对象，减少内存分配的次数
// Example:
// ```
// p = sync.NewPool(func() {
// return make(map[string]string)
// })
// m = p.Get() // 从 Pool 中获取，如果 Pool 中没有，则会调用传入的第一个参数函数，返回一个新的 map[string]string
// m["1"] = "2"
// println(m) // {"1": "2"}
// // 将 m 放回 Pool 中
// p.Put(m)
// m2 = p.Get() // 从 Pool 中获取，实际上我们获取到的是刚 Put 进去的 m
// println(m2) // {"1": "2"}
// ```
func NewPool(newFunc ...func() any) *sync.Pool {
	if len(newFunc) > 0 {
		return &sync.Pool{
			New: newFunc[0],
		}
	}
	return new(sync.Pool)
}

// NewCond 创建一个 Cond 结构体引用，即一个条件变量，参考golang官方文档：https://golang.org/pkg/sync/#Cond
// 条件变量是一种用于协调多个并发任务之间的同步机制，它允许一个任务等待某个条件成立，同时允许其他任务在条件成立时通知等待的任务
// Example:
// ```
// c = sync.NewCond()
// done = false
// func read(name) {
// c.L.Lock()
// for !done {
// c.Wait()
// }
// println(name, "start reading")
// c.L.Unlock()
// }
//
// func write(name) {
// time.sleep(1)
// println(name, "start writing")
// c.L.Lock()
// done = true
// c.L.Unlock()
// println(name, "wakes all")
// c.Broadcast()
// }
//
// go read("reader1")
// go read("reader2")
// go read("reader3")
// write("writer")
// time.sleep(3)
// ```
func NewCond() *sync.Cond {
	return sync.NewCond(new(sync.Mutex))
}

var SyncExport = map[string]interface{}{
	"NewWaitGroup":      NewWaitGroup,
	"NewSizedWaitGroup": NewSizedWaitGroup,
	"NewMutex":          NewMutex,
	"NewLock":           NewLock,
	"NewMap":            NewMap,
	"NewOnce":           NewOnce,
	"NewRWMutex":        NewRWMutex,
	"NewPool":           NewPool,
	"NewCond":           NewCond,
}

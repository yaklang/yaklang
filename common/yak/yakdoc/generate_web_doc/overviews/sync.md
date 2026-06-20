`sync` 库是 Go 标准库 `sync` 的 yak 封装，提供并发原语：互斥锁、读写锁、等待组、once、条件变量、并发安全 map 与对象池，是编写并发脚本的基础设施。

典型使用场景：

- 锁：`sync.NewMutex` / `sync.NewLock` 互斥锁，`sync.NewRWMutex` 读写锁，`sync.NewCond` 条件变量。
- 协作：`sync.NewWaitGroup` 等待一组协程结束，`sync.NewSizedWaitGroup` 带并发上限的等待组（常用于控制扫描并发），`sync.NewOnce` 只执行一次。
- 容器：`sync.NewMap` 并发安全 map，`sync.NewPool` 对象池。

与相邻库的关系：`sync` 是并发基础库，常与 `context`（取消控制）配合，在批量扫描、并发请求等场景中控制并发度与同步。

package lsp

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// AnalysisTask 表示一个分析任务
type AnalysisTask struct {
	URI        string
	Priority   AnalysisPriority
	ScriptType string
	Callback   func(*ssaapi.Program, error)
}

// AnalysisPriority 分析优先级
type AnalysisPriority int

const (
	// PriorityLow 低优先级（后台分析）
	PriorityLow AnalysisPriority = iota
	// PriorityNormal 普通优先级（停顿后触发）
	PriorityNormal
	// PriorityHigh 高优先级（保存或请求驱动）
	PriorityHigh
	// PriorityImmediate 立即优先级（阻塞等待）
	PriorityImmediate
)

// EditScheduler 管理编辑后的分析调度
type EditScheduler struct {
	docMgr *DocumentManager

	// Debounce 定时器（每个文档一个）
	timers   map[string]*time.Timer
	timersMu sync.Mutex

	// 任务队列
	tasks   chan *AnalysisTask
	workers int

	// 上下文控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 配置参数
	shortDebounce time.Duration // 短暂停（语法分析）
	longDebounce  time.Duration // 长暂停（SSA 分析）
}

// NewEditScheduler 创建编辑调度器
func NewEditScheduler(docMgr *DocumentManager) *EditScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	es := &EditScheduler{
		docMgr:        docMgr,
		timers:        make(map[string]*time.Timer),
		tasks:         make(chan *AnalysisTask, 100),
		workers:       4, // 并发工作线程数
		ctx:           ctx,
		cancel:        cancel,
		shortDebounce: 400 * time.Millisecond,  // 短暂停触发语法分析
		longDebounce:  1500 * time.Millisecond, // 长暂停触发 SSA 分析
	}

	// 启动工作线程
	for i := 0; i < es.workers; i++ {
		es.wg.Add(1)
		go es.worker()
	}

	return es
}

// Stop 停止调度器
func (es *EditScheduler) Stop() {
	es.cancel()
	close(es.tasks)
	es.wg.Wait()

	// 停止所有定时器
	es.timersMu.Lock()
	for _, timer := range es.timers {
		timer.Stop()
	}
	es.timers = make(map[string]*time.Timer)
	es.timersMu.Unlock()
}

// ScheduleAnalysis 调度分析任务（debounce）
func (es *EditScheduler) ScheduleAnalysis(uri string, scriptType string) {
	doc, exists := es.docMgr.GetDocument(uri)
	if !exists {
		return
	}

	// 取消之前的定时器
	es.timersMu.Lock()
	if timer, exists := es.timers[uri]; exists {
		timer.Stop()
	}

	// 判断使用哪个 debounce 时间
	debounce := es.longDebounce
	if doc.IsTyping() {
		// 输入爆发状态，使用更长的 debounce
		debounce = es.longDebounce
	} else {
		// 停顿后的单次编辑，可以更快触发
		debounce = es.shortDebounce
	}

	// 创建新定时器
	timer := time.AfterFunc(debounce, func() {
		es.triggerAnalysis(uri, scriptType, PriorityNormal)
	})
	es.timers[uri] = timer
	es.timersMu.Unlock()

	log.Debugf("[LSP Scheduler] scheduled analysis for %s (debounce: %v)", uri, debounce)
}

// ScheduleImmediateAnalysis 立即调度分析（保存或请求驱动）
func (es *EditScheduler) ScheduleImmediateAnalysis(uri string, scriptType string) {
	// 取消 debounce 定时器
	es.timersMu.Lock()
	if timer, exists := es.timers[uri]; exists {
		timer.Stop()
		delete(es.timers, uri)
	}
	es.timersMu.Unlock()

	es.triggerAnalysis(uri, scriptType, PriorityHigh)
}

// triggerAnalysis 触发分析任务
func (es *EditScheduler) triggerAnalysis(uri string, scriptType string, priority AnalysisPriority) {
	task := &AnalysisTask{
		URI:        uri,
		Priority:   priority,
		ScriptType: scriptType,
	}

	select {
	case es.tasks <- task:
		log.Debugf("[LSP Scheduler] triggered analysis for %s (priority: %d)", uri, priority)
	case <-es.ctx.Done():
		return
	default:
		// 队列满，跳过
		log.Warnf("[LSP Scheduler] task queue full, skipping analysis for %s", uri)
	}
}

// worker 工作线程
func (es *EditScheduler) worker() {
	defer es.wg.Done()

	for {
		select {
		case task, ok := <-es.tasks:
			if !ok {
				return
			}
			es.processTask(task)
		case <-es.ctx.Done():
			return
		}
	}
}

// processTask 处理分析任务
func (es *EditScheduler) processTask(task *AnalysisTask) {
	doc, exists := es.docMgr.GetDocument(task.URI)
	if !exists {
		log.Warnf("[LSP Scheduler] document not found: %s", task.URI)
		return
	}

	content := doc.GetContent()
	log.Debugf("[LSP Scheduler] processing analysis task for %s (size: %d bytes)", task.URI, len(content))

	// 计算新的哈希
	newHash := ComputeCodeHash(content)

	// 检查是否需要重新编译
	ssaCache := doc.GetSSACache()
	if ssaCache != nil && ssaCache.Hash == newHash.Semantic {
		// 语义哈希相同，不需要重新编译
		log.Debugf("[LSP Scheduler] semantic hash unchanged, skipping SSA compilation for %s", task.URI)
		if ssaCache.Stale {
			// 更新 stale 标记
			doc.mu.Lock()
			ssaCache.Stale = false
			doc.mu.Unlock()
		}
		return
	}

	// 检查 AST 是否需要重新解析
	syntaxCache := doc.GetSyntaxCache()
	needReparseAST := true
	if syntaxCache != nil && syntaxCache.Hash == newHash.Structure {
		needReparseAST = false
		log.Debugf("[LSP Scheduler] structure hash unchanged, reusing AST for %s", task.URI)
	}

	// 执行编译
	start := time.Now()
	prog, err := es.compileDocument(content, task.ScriptType, needReparseAST, syntaxCache)
	duration := time.Since(start)

	if err != nil {
		log.Errorf("[LSP Scheduler] compilation failed for %s: %v (took %v)", task.URI, err, duration)
		if task.Callback != nil {
			task.Callback(nil, err)
		}
		return
	}

	// 更新缓存
	doc.SetSSACache(prog, newHash.Semantic)
	log.Infof("[LSP Scheduler] compiled %s successfully (took %v)", task.URI, duration)

	if task.Callback != nil {
		task.Callback(prog, nil)
	}
}

// compileDocument 编译文档
func (es *EditScheduler) compileDocument(content string, scriptType string, needReparseAST bool, syntaxCache *SyntaxCache) (*ssaapi.Program, error) {
	// 复用现有的编译逻辑
	opts := []ssaapi.Option{
		ssaapi.WithEnableCache(),
	}

	// 如果有 AntlrCache 可以复用
	if syntaxCache != nil && syntaxCache.AntlrCache != nil && !needReparseAST {
		// 注意：这里需要确认 ssaapi 是否支持传入 AntlrCache
		// 当前先使用标准编译，未来可以优化
		log.Debugf("[LSP Scheduler] reusing antlr cache (future optimization)")
	}

	// 编译
	prog, err := ssaapi.Parse(content, opts...)
	return prog, err
}

// GetPendingTaskCount 获取待处理任务数
func (es *EditScheduler) GetPendingTaskCount() int {
	return len(es.tasks)
}

// RequestDrivenAnalysis 请求驱动的分析（阻塞等待结果）
func (es *EditScheduler) RequestDrivenAnalysis(uri string, scriptType string, timeout time.Duration) (*ssaapi.Program, error) {
	doc, exists := es.docMgr.GetDocument(uri)
	if !exists {
		return nil, nil
	}

	// 检查缓存
	ssaCache := doc.GetSSACache()
	content := doc.GetContent()
	newHash := ComputeCodeHash(content)

	if ssaCache != nil && ssaCache.Hash == newHash.Semantic {
		// 缓存命中
		log.Debugf("[LSP Scheduler] cache hit for request-driven analysis: %s", uri)
		return ssaCache.Program, nil
	}

	// 缓存未命中，需要编译
	log.Debugf("[LSP Scheduler] cache miss, compiling for request: %s", uri)

	// 使用通道等待结果
	resultCh := make(chan struct {
		prog *ssaapi.Program
		err  error
	}, 1)

	task := &AnalysisTask{
		URI:        uri,
		Priority:   PriorityImmediate,
		ScriptType: scriptType,
		Callback: func(prog *ssaapi.Program, err error) {
			resultCh <- struct {
				prog *ssaapi.Program
				err  error
			}{prog, err}
		},
	}

	// 立即提交任务
	select {
	case es.tasks <- task:
	case <-time.After(timeout):
		return nil, context.DeadlineExceeded
	}

	// 等待结果
	select {
	case result := <-resultCh:
		return result.prog, result.err
	case <-time.After(timeout):
		return nil, context.DeadlineExceeded
	}
}

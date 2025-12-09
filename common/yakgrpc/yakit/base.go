package yakit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// SQLTraceLogger 用于捕获最后一条 SQL 日志（gorm v1）
type SQLTraceLogger struct {
	mu   sync.Mutex
	last string
}

func (l *SQLTraceLogger) Print(v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = utils.ShrinkString(fmt.Sprint(v...), 512)
}

func (l *SQLTraceLogger) Last() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

// sqlTraceLogger 用于内部使用（保持向后兼容）
type sqlTraceLogger = SQLTraceLogger

// LongSQLDescription 描述慢 SQL 的详细信息
type LongSQLDescription struct {
	Duration      time.Duration `json:"duration"`       // SQL 执行耗时
	DurationMs    int64          `json:"duration_ms"`   // SQL 执行耗时（毫秒）
	DurationStr   string         `json:"duration_str"`   // SQL 执行耗时（字符串形式）
	FuncName      string         `json:"func_name"`      // 函数名
	FuncPtr       string         `json:"func_ptr"`       // 函数指针（字符串形式）
	QueueLen      int            `json:"queue_len"`      // 队列长度
	LastSQL       string         `json:"last_sql"`      // 最后执行的 SQL
	Timestamp     time.Time      `json:"timestamp"`     // 时间戳
	TimestampUnix int64          `json:"timestamp_unix"` // 时间戳（Unix 秒）
}

// HTTPFlowSQLCallback 慢 SQL 回调函数类型
type HTTPFlowSQLCallback func(avgCost time.Duration, items []*LongSQLDescription)

// SlowRuleHookDescription 描述慢规则 Hook 的详细信息
type SlowRuleHookDescription struct {
	Duration      time.Duration `json:"duration"`       // Hook 执行耗时
	DurationMs    int64         `json:"duration_ms"`   // Hook 执行耗时（毫秒）
	DurationStr   string        `json:"duration_str"`   // Hook 执行耗时（字符串形式）
	HookType      string        `json:"hook_type"`      // Hook 类型：hook_color, hook_request, hook_response
	RuleCount     int           `json:"rule_count"`     // 当前规则数量
	URL           string        `json:"url"`            // 请求 URL（如果有）
	Timestamp     time.Time     `json:"timestamp"`      // 时间戳
	TimestampUnix int64         `json:"timestamp_unix"` // 时间戳（Unix 秒）
}

// MITMSlowRuleHookCallback 慢规则 Hook 回调函数类型
type MITMSlowRuleHookCallback func(avgCost time.Duration, items []*SlowRuleHookDescription)

type initializingCallback struct {
	Note string
	Fn   func() error
}

var (
	__initializingDatabase []*initializingCallback
	__mutexForInit         = new(sync.Mutex)

	// HTTPFlowSlowInsertCallback 相关变量（慢插入）
	httpFlowSlowInsertCallbackMutex sync.Mutex
	httpFlowSlowInsertCallback      HTTPFlowSQLCallback
	slowInsertSQLItems              []*LongSQLDescription // 收集慢插入 SQL 信息
	slowInsertSQLItemsMutex         sync.Mutex
	slowInsertSQLThrottle           = utils.NewThrottle(2) // 每2秒最多触发一次

	// HTTPFlowSlowQueryCallback 相关变量（慢查询）
	httpFlowSlowQueryCallbackMutex sync.Mutex
	httpFlowSlowQueryCallback      HTTPFlowSQLCallback
	slowQuerySQLItems              []*LongSQLDescription // 收集慢查询 SQL 信息
	slowQuerySQLItemsMutex         sync.Mutex
	slowQuerySQLThrottle           = utils.NewThrottle(2) // 每2秒最多触发一次

	// MITMSlowRuleHookCallback 相关变量（慢规则 Hook）
	mitmSlowRuleHookCallbackMutex sync.Mutex
	mitmSlowRuleHookCallback     MITMSlowRuleHookCallback
	slowRuleHookItems            []*SlowRuleHookDescription // 收集慢规则 Hook 信息
	slowRuleHookItemsMutex       sync.Mutex
	slowRuleHookThrottle          = utils.NewThrottle(2) // 每2秒最多触发一次
)

type DbExecFunc func(db *gorm.DB) error

var DBSaveAsyncChannel = make(chan DbExecFunc, 40960)

func init() {
	throttle := utils.NewThrottle(2)
	go func() {
		var count uint64 = 0
		for {
			select {
			case f := <-DBSaveAsyncChannel:
				start := time.Now()
				// 为单次执行创建 tracer，记录耗时 SQL
				tracer := &sqlTraceLogger{}
				db := consts.GetGormProjectDatabase()
				if db != nil {
					// SetLogger 可能无返回值，分开调用以兼容
					db.SetLogger(tracer)
					db = db.LogMode(true)
				}

				err := f(db)
				elapsed := time.Since(start)
				if elapsed > 2*time.Second {
					// 提供更多信息帮助定位耗时的执行
					ptr := reflect.ValueOf(f).Pointer()
					fn := runtime.FuncForPC(ptr)
					fnName := "<unknown>"
					if fn != nil {
						fnName = fn.Name()
					}
					log.Warnf("SQL execution took too long: %v, func_ptr:%p, func_name:%s, queue_len:%d, last_sql:%s",
						elapsed, f, fnName, len(DBSaveAsyncChannel), tracer.Last())

					// 收集慢 SQL 信息
					now := time.Now()
					slowSQLItem := &LongSQLDescription{
						Duration:      elapsed,
						DurationMs:    elapsed.Milliseconds(),
						DurationStr:   elapsed.String(),
						FuncName:      fnName,
						FuncPtr:       fmt.Sprintf("%p", f),
						QueueLen:      len(DBSaveAsyncChannel),
						LastSQL:       tracer.Last(),
						Timestamp:     now,
						TimestampUnix: now.Unix(),
					}

					// 添加到收集列表（慢插入）
					slowInsertSQLItemsMutex.Lock()
					slowInsertSQLItems = append(slowInsertSQLItems, slowSQLItem)
					slowInsertSQLItemsMutex.Unlock()

					// 使用节流机制触发回调（每2秒最多触发一次），异步执行
					slowInsertSQLThrottle(func() {
						go triggerSlowInsertSQLCallback()
					})
				}
				count++
				if count%1000 == 0 {
					throttle(func() {
						//log.Infof("Throttle sql exec count: %d", count)
					})
				}
				if err != nil {
					log.Errorf("Throttle sql exec failed: %s", err)
				}
			}
		}
	}()
}

// triggerSlowInsertSQLCallback 触发慢插入 SQL 回调
func triggerSlowInsertSQLCallback() {
	httpFlowSlowInsertCallbackMutex.Lock()
	callback := httpFlowSlowInsertCallback
	httpFlowSlowInsertCallbackMutex.Unlock()

	if callback == nil {
		return
	}

	// 获取并清空收集的慢插入 SQL 信息
	slowInsertSQLItemsMutex.Lock()
	items := make([]*LongSQLDescription, len(slowInsertSQLItems))
	copy(items, slowInsertSQLItems)
	slowInsertSQLItems = slowInsertSQLItems[:0] // 清空列表
	slowInsertSQLItemsMutex.Unlock()

	if len(items) == 0 {
		return
	}

	// 计算平均耗时
	var totalDuration time.Duration
	for _, item := range items {
		totalDuration += item.Duration
	}
	avgCost := totalDuration / time.Duration(len(items))

	// 调用回调
	callback(avgCost, items)
}

// triggerSlowQuerySQLCallback 触发慢查询 SQL 回调
func triggerSlowQuerySQLCallback() {
	httpFlowSlowQueryCallbackMutex.Lock()
	callback := httpFlowSlowQueryCallback
	httpFlowSlowQueryCallbackMutex.Unlock()

	if callback == nil {
		return
	}

	// 获取并清空收集的慢查询 SQL 信息
	slowQuerySQLItemsMutex.Lock()
	items := make([]*LongSQLDescription, len(slowQuerySQLItems))
	copy(items, slowQuerySQLItems)
	slowQuerySQLItems = slowQuerySQLItems[:0] // 清空列表
	slowQuerySQLItemsMutex.Unlock()

	if len(items) == 0 {
		return
	}

	// 计算平均耗时
	var totalDuration time.Duration
	for _, item := range items {
		totalDuration += item.Duration
	}
	avgCost := totalDuration / time.Duration(len(items))

	// 调用回调
	callback(avgCost, items)
}

// RegisterHTTPFlowSlowInsertCallback 注册 HTTPFlow SQL 慢插入回调
// callback 在慢插入 SQL 出现时触发，每2秒最多触发一次
func RegisterHTTPFlowSlowInsertCallback(callback HTTPFlowSQLCallback) {
	httpFlowSlowInsertCallbackMutex.Lock()
	defer httpFlowSlowInsertCallbackMutex.Unlock()
	httpFlowSlowInsertCallback = callback
}

// RegisterHTTPFlowSlowQueryCallback 注册 HTTPFlow SQL 慢查询回调
// callback 在慢查询 SQL 出现时触发，每2秒最多触发一次
func RegisterHTTPFlowSlowQueryCallback(callback HTTPFlowSQLCallback) {
	httpFlowSlowQueryCallbackMutex.Lock()
	defer httpFlowSlowQueryCallbackMutex.Unlock()
	httpFlowSlowQueryCallback = callback
}

// MockHTTPFlowSlowInsertSQL 模拟一次慢插入 SQL 执行，用于测试
// duration 指定模拟的 SQL 执行耗时
func MockHTTPFlowSlowInsertSQL(duration time.Duration) {
	if duration < 2*time.Second {
		duration = 2*time.Second + 100*time.Millisecond // 确保超过阈值
	}

	// 创建一个模拟的慢插入 SQL 项
	now := time.Now()
	slowSQLItem := &LongSQLDescription{
		Duration:      duration,
		DurationMs:    duration.Milliseconds(),
		DurationStr:   duration.String(),
		FuncName:      "yakit.MockHTTPFlowSlowInsertSQL",
		FuncPtr:       "mock",
		QueueLen:      len(DBSaveAsyncChannel),
		LastSQL:       "MOCK SQL: INSERT INTO http_flows ...",
		Timestamp:     now,
		TimestampUnix: now.Unix(),
	}

	// 添加到收集列表
	slowInsertSQLItemsMutex.Lock()
	slowInsertSQLItems = append(slowInsertSQLItems, slowSQLItem)
	slowInsertSQLItemsMutex.Unlock()

	// 异步触发回调
	go triggerSlowInsertSQLCallback()
}

// AddSlowQuerySQLItem 添加慢查询 SQL 项（线程安全）
func AddSlowQuerySQLItem(item *LongSQLDescription) {
	slowQuerySQLItemsMutex.Lock()
	defer slowQuerySQLItemsMutex.Unlock()
	slowQuerySQLItems = append(slowQuerySQLItems, item)
}

// TriggerSlowQuerySQLCallbackThrottled 使用节流机制触发慢查询回调
func TriggerSlowQuerySQLCallbackThrottled() {
	slowQuerySQLThrottle(func() {
		go triggerSlowQuerySQLCallback()
	})
}

// MockHTTPFlowSlowQuerySQL 模拟一次慢查询 SQL 执行，用于测试
// duration 指定模拟的 SQL 执行耗时
func MockHTTPFlowSlowQuerySQL(duration time.Duration) {
	if duration < 2*time.Second {
		duration = 2*time.Second + 100*time.Millisecond // 确保超过阈值
	}

	// 创建一个模拟的慢查询 SQL 项
	now := time.Now()
	slowSQLItem := &LongSQLDescription{
		Duration:      duration,
		DurationMs:    duration.Milliseconds(),
		DurationStr:   duration.String(),
		FuncName:      "yakit.MockHTTPFlowSlowQuerySQL",
		FuncPtr:       "mock",
		QueueLen:      0, // 查询操作没有队列
		LastSQL:       "MOCK SQL: SELECT * FROM http_flows ...",
		Timestamp:     now,
		TimestampUnix: now.Unix(),
	}

	// 添加到收集列表
	AddSlowQuerySQLItem(slowSQLItem)

	// 使用节流机制触发回调
	TriggerSlowQuerySQLCallbackThrottled()
}

// triggerSlowRuleHookCallback 触发慢规则 Hook 回调
func triggerSlowRuleHookCallback() {
	mitmSlowRuleHookCallbackMutex.Lock()
	callback := mitmSlowRuleHookCallback
	mitmSlowRuleHookCallbackMutex.Unlock()

	if callback == nil {
		return
	}

	// 获取并清空收集的慢规则 Hook 信息
	slowRuleHookItemsMutex.Lock()
	items := make([]*SlowRuleHookDescription, len(slowRuleHookItems))
	copy(items, slowRuleHookItems)
	slowRuleHookItems = slowRuleHookItems[:0] // 清空列表
	slowRuleHookItemsMutex.Unlock()

	if len(items) == 0 {
		return
	}

	// 计算平均耗时
	var totalDuration time.Duration
	for _, item := range items {
		totalDuration += item.Duration
	}
	avgCost := totalDuration / time.Duration(len(items))

	// 调用回调
	callback(avgCost, items)
}

// RegisterMITMSlowRuleHookCallback 注册 MITM 慢规则 Hook 回调
// callback 在慢规则 Hook 出现时触发，每2秒最多触发一次
func RegisterMITMSlowRuleHookCallback(callback MITMSlowRuleHookCallback) {
	mitmSlowRuleHookCallbackMutex.Lock()
	defer mitmSlowRuleHookCallbackMutex.Unlock()
	mitmSlowRuleHookCallback = callback
}

// AddSlowRuleHookItem 添加慢规则 Hook 项（线程安全）
func AddSlowRuleHookItem(item *SlowRuleHookDescription) {
	slowRuleHookItemsMutex.Lock()
	defer slowRuleHookItemsMutex.Unlock()
	slowRuleHookItems = append(slowRuleHookItems, item)
}

// TriggerSlowRuleHookCallbackThrottled 使用节流机制触发慢规则 Hook 回调
func TriggerSlowRuleHookCallbackThrottled() {
	slowRuleHookThrottle(func() {
		go triggerSlowRuleHookCallback()
	})
}

// MockMITMSlowRuleHook 模拟一次慢规则 Hook 执行，用于测试
// duration 指定模拟的 Hook 执行耗时
// hookType 指定 Hook 类型：hook_color, hook_request, hook_response
// ruleCount 指定规则数量
func MockMITMSlowRuleHook(duration time.Duration, hookType string, ruleCount int) {
	if duration < 300*time.Millisecond {
		duration = 300*time.Millisecond + 100*time.Millisecond // 确保超过阈值
	}

	// 创建一个模拟的慢规则 Hook 项
	now := time.Now()
	slowHookItem := &SlowRuleHookDescription{
		Duration:      duration,
		DurationMs:    duration.Milliseconds(),
		DurationStr:   duration.String(),
		HookType:      hookType,
		RuleCount:     ruleCount,
		URL:           "http://mock.example.com/test",
		Timestamp:     now,
		TimestampUnix: now.Unix(),
	}

	// 添加到收集列表
	AddSlowRuleHookItem(slowHookItem)

	// 使用节流机制触发回调
	TriggerSlowRuleHookCallbackThrottled()
}

func RegisterPostInitDatabaseFunction(f func() error, notes ...string) {
	__mutexForInit.Lock()
	defer __mutexForInit.Unlock()
	__initializingDatabase = append(__initializingDatabase, &initializingCallback{
		Note: strings.Join(notes, ";"),
		Fn: func() error {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("PostInitDatabaseFunction panic: %v\n%s", r, spew.Sdump(r))
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			return f()
		},
	})
}

const md5Placeholder = "f97f966eb7f8ba8fdb63e4d29109c058" // md5("CallPostInitDatabase")

func CallPostInitDatabase() error {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			// if the context timed out, echo a warning for frontend
			msg := "CallPostInitDatabase is taking too long, please wait..."

			// log for frontend
			m := make(map[string]any)
			m["warning"] = msg
			msgBytes, _ := json.Marshal(m)
			log.Warnf("<json-%s>%s<json-%s>\n",
				md5Placeholder, string(msgBytes), md5Placeholder,
			)

			// log for console
			log.Warn(msg)
		}
	}()

	for idx, f := range __initializingDatabase {
		if f == nil || f.Fn == nil {
			continue
		}
		currentFuncStart := time.Now()
		err := f.Fn()
		elapsed := time.Since(currentFuncStart)
		if elapsed > 1*time.Second {
			node := f.Note
			if node == "" {
				node = fmt.Sprint(idx)
			}
			log.Warnf("call post function[%v] took too long: %v", node, elapsed)
		}
		if err != nil {
			return err
		}
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		log.Warnf("call post functions took too long: %v", elapsed)
	}
	return nil
}

func InitialDatabase() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
	err := CallPostInitDatabase()
	if err != nil {
		log.Errorf(`yakit.CallPostInitDatabase failed: %s`, err)
	}
}

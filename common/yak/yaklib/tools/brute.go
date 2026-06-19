package tools

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var BruterExports = map[string]interface{}{
	"GetAvailableBruteTypes":       GetAvailableBruteTypes,
	"GetUsernameListFromBruteType": GetUsernameListFromBruteType,
	"GetPasswordListFromBruteType": GetPasswordListFromBruteType,

	"New":                _yakitBruterNew,
	"concurrentTarget":   yakBruteOpt_ConcurrentTarget,
	"debug":              yakBruteOpt_Debug,
	"userList":           yakBruteOpt_userlist,
	"passList":           yakBruteOpt_passlist,
	"autoDict":           yakBruteOpt_autoDict,
	"concurrent":         yakBruteOpt_concurrent,
	"minDelay":           yakBruteOpt_minDelay,
	"maxDelay":           yakBruteOpt_maxDelay,
	"bruteHandler":       yakBruteOpt_coreHandler,
	"okToStop":           yakBruteOpt_OkToStop,
	"finishingThreshold": yakBruteOpt_FinishingThreshold,
}

// GetAvailableBruteTypes 返回当前支持的所有内置爆破类型(协议/服务)名称列表
// 在 yak 中通过 brute.GetAvailableBruteTypes 调用
// 返回值:
//   - 支持的爆破类型名称字符串切片，如 ssh、ftp、redis、mysql 等
//
// Example:
// ```
// types = brute.GetAvailableBruteTypes()
// assert len(types) > 0, "should expose builtin brute types"
// // 常见服务 ssh 应在支持列表中
// println("ssh" in types)   // OUT: true
// assert "ssh" in types, "ssh brute type should be available"
// ```
func GetAvailableBruteTypes() []string {
	return bruteutils.GetBuildinAvailableBruteType()
}

// GetUsernameListFromBruteType 返回指定爆破类型对应的内置用户名字典
// 在 yak 中通过 brute.GetUsernameListFromBruteType 调用
// 参数:
//   - t: 爆破类型名称，如 ssh、mysql
//
// 返回值:
//   - 该类型的内置用户名候选列表
//
// Example:
// ```
// users = brute.GetUsernameListFromBruteType("ssh")
// assert len(users) > 0, "ssh username dict should not be empty"
// ```
func GetUsernameListFromBruteType(t string) []string {
	return bruteutils.GetUsernameListFromBruteType(t)
}

// GetPasswordListFromBruteType 返回指定爆破类型对应的内置密码字典
// 在 yak 中通过 brute.GetPasswordListFromBruteType 调用
// 参数:
//   - t: 爆破类型名称，如 ssh、mysql
//
// 返回值:
//   - 该类型的内置密码候选列表
//
// Example:
// ```
// passwords = brute.GetPasswordListFromBruteType("ssh")
// assert len(passwords) > 0, "ssh password dict should not be empty"
// ```
func GetPasswordListFromBruteType(t string) []string {
	return bruteutils.GetPasswordListFromBruteType(t)
}

type yakBruter struct {
	Ctx       context.Context
	RuntimeId string
	debug     bool `json:"debug"`

	// 设置用户与密码爆破字典
	userList []string
	passList []string

	// 设置爆破相关处理函数
	bruteType     string `json:"brute_type"`
	coreHandler   bruteutils.BruteCallback
	resultHandler func(res *bruteutils.BruteItemResult)

	// 同时支持多少个并发目标同时测试？默认 256
	concurrentTarget int

	// 同时每个目标的并发
	concurrent int

	// 每个目标每两次测试之间间隔最小
	minDelay int

	// 每个目标每两次测试间隔最大
	maxDelay int

	// okToStop
	okToStop bool

	// 完成阈值
	finishingThreshold int
}

type BruteOpt func(bruter *yakBruter)

func WithBruteCtx(ctx context.Context) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.Ctx = ctx
	}
}

func WithBruteRuntimeId(id string) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.RuntimeId = id
	}
}

// debug 设置爆破器是否开启调试模式，开启后会输出更详细的过程日志
// 在 yak 中通过 brute.debug 调用
// 参数:
//   - b: 是否开启调试
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：开启调试模式
// bruter = brute.New("ssh", brute.debug(true))~
// ```
func yakBruteOpt_Debug(b bool) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.debug = b
	}
}

// okToStop 设置当某个目标爆破出有效凭据后是否立即停止对该目标的后续尝试
// 在 yak 中通过 brute.okToStop 调用
// 参数:
//   - b: 命中后是否停止
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：命中即停止
// bruter = brute.New("ssh", brute.okToStop(true))~
// ```
func yakBruteOpt_OkToStop(b bool) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.okToStop = b
	}
}

// finishingThreshold 设置单个目标的失败容忍阈值，连续失败达到该值后提前结束该目标的爆破
// 在 yak 中通过 brute.finishingThreshold 调用
// 参数:
//   - i: 失败阈值
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置失败阈值
// bruter = brute.New("ssh", brute.finishingThreshold(50))~
// ```
func yakBruteOpt_FinishingThreshold(i int) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.finishingThreshold = i
	}
}

// concurrentTarget 设置同时进行爆破的目标数量(默认 256)
// 在 yak 中通过 brute.concurrentTarget 调用
// 参数:
//   - c: 并发目标数量
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：同时爆破 10 个目标
// bruter = brute.New("ssh", brute.concurrentTarget(10))~
// ```
func yakBruteOpt_ConcurrentTarget(c int) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.concurrentTarget = c
	}
}

// userList 设置爆破使用的用户名字典
// 在 yak 中通过 brute.userList 调用
// 参数:
//   - users: 一个或多个用户名
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定用户名字典
// bruter = brute.New("ssh", brute.userList("root", "admin"))~
// ```
func yakBruteOpt_userlist(users ...string) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.userList = users
	}
}

// autoDict 根据爆破类型自动加载内置的用户名与密码字典
// 在 yak 中通过 brute.autoDict 调用
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：自动使用内置字典
// bruter = brute.New("ssh", brute.autoDict())~
// ```
func yakBruteOpt_autoDict() BruteOpt {
	auto := true
	return func(bruter *yakBruter) {
		if auto {
			bruter.userList = bruteutils.GetUsernameListFromBruteType(bruter.bruteType)
			bruter.passList = bruteutils.GetPasswordListFromBruteType(bruter.bruteType)
		}
	}
}

// passList 设置爆破使用的密码字典
// 在 yak 中通过 brute.passList 调用
// 参数:
//   - passes: 一个或多个密码
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定密码字典
// bruter = brute.New("ssh", brute.passList("123456", "password"))~
// ```
func yakBruteOpt_passlist(passes ...string) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.passList = passes
	}
}

// minDelay 设置每个目标两次尝试之间的最小间隔秒数
// 在 yak 中通过 brute.minDelay 调用
// 参数:
//   - min: 最小间隔(秒)
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置最小间隔 1 秒
// bruter = brute.New("ssh", brute.minDelay(1))~
// ```
func yakBruteOpt_minDelay(min int) BruteOpt {
	return func(b *yakBruter) {
		b.minDelay = min
	}
}

// concurrent 设置单个目标内部的并发尝试数量
// 在 yak 中通过 brute.concurrent 调用
// 参数:
//   - c: 单目标并发数
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：单目标并发设为 1
// bruter = brute.New("ssh", brute.concurrent(1))~
// ```
func yakBruteOpt_concurrent(c int) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.concurrent = c
	}
}

// maxDelay 设置每个目标两次尝试之间的最大间隔秒数
// 在 yak 中通过 brute.maxDelay 调用
// 参数:
//   - max: 最大间隔(秒)
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置最大间隔 5 秒
// bruter = brute.New("ssh", brute.maxDelay(5))~
// ```
func yakBruteOpt_maxDelay(max int) BruteOpt {
	return func(b *yakBruter) {
		b.maxDelay = max
	}
}

// bruteHandler 自定义爆破的核心处理函数，覆盖默认的协议爆破逻辑
// 在 yak 中通过 brute.bruteHandler 调用
// 参数:
//   - cb: 处理单个爆破项并返回结果的回调函数
//
// 返回值:
//   - 一个 brute.New 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：自定义爆破处理逻辑
//
//	bruter = brute.New("ssh", brute.bruteHandler(func(item) {
//	    return item.Result()
//	}))~
//
// ```
func yakBruteOpt_coreHandler(cb func(item *bruteutils.BruteItem) *bruteutils.BruteItemResult) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.coreHandler = cb
	}
}

func (y *yakBruter) Start(targets ...string) (chan *bruteutils.BruteItemResult, error) {
	action, err := bruteutils.WithDelayerWaiter(y.minDelay, y.maxDelay)
	if err != nil {
		action, _ = bruteutils.WithDelayerWaiter(1, 5)
	}

	if len(targets) <= 0 {
		return nil, utils.Errorf("empty targets for %v", y.bruteType)
	}

	bruter, err := bruteutils.NewMultiTargetBruteUtilEx(
		bruteutils.WithBruteCallback(y.coreHandler),
		bruteutils.WithTargetsConcurrent(y.concurrentTarget),
		bruteutils.WithTargetTasksConcurrent(y.concurrent),
		bruteutils.WithOkToStop(y.okToStop),
		bruteutils.WithFinishingThreshold(y.finishingThreshold),
		action,
	)
	if err != nil {
		return nil, utils.Errorf("create core bruter[%v] failed: %s", y.bruteType, err.Error())
	}

	ch := make(chan *bruteutils.BruteItemResult, 100)
	go func() {
		defer close(ch)

		if funk.IsEmpty(y.userList) {
			y.userList = []string{""}
		}
		if funk.IsEmpty(y.passList) {
			y.passList = []string{""}
		}

		err := bruter.StreamBruteContext(y.Ctx, y.bruteType, targets, y.userList, y.passList, func(b *bruteutils.BruteItemResult) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			select {
			case <-y.Ctx.Done():
				return
			case ch <- b:
				if !b.Ok {
					return
				}
				if err := updateHitCount(b); err != nil {
					log.Errorf("update hit count failed: %v", err)
				}
			}
		})
		if err != nil {
			log.Errorf("build stream context failed: %s", err.Error())
			return
		}
	}()

	return ch, nil
}

func updateHitCount(b *bruteutils.BruteItemResult) error {
	db := consts.GetGormProfileDatabase()

	// 更新用户名命中次数
	if err := updatePayloadHitCount(db, b.Username); err != nil {
		return err
	}

	// 更新密码命中次数
	if err := updatePayloadHitCount(db, b.Password); err != nil {
		return err
	}

	return nil
}

func updatePayloadHitCount(db *gorm.DB, payload string) error {
	payloads, err := yakit.QueryPayloadWithoutPaging(db, "", "", payload)
	if err != nil {
		return err
	}

	if len(payloads) > 0 {
		var hitCount int64 = 0
		for _, p := range payloads {
			if p.HitCount == nil {
				p.HitCount = &hitCount
			} else {
				hitCount = *p.HitCount
			}
			hitCount++
			p.HitCount = &hitCount
			if err := yakit.UpdatePayload(db, int(p.ID), p); err != nil {
				return err
			}
		}
	}
	return nil
}

// New 创建一个指定类型的弱口令爆破器，可通过选项配置字典、并发、延迟等，再调用 Start 对目标执行爆破
// 在 yak 中通过 brute.New 调用
// 参数:
//   - typeStr: 爆破类型名称，如 ssh、mysql、redis
//   - opts: 可选配置项，如 brute.userList、brute.passList、brute.concurrent 等
//
// 返回值:
//   - 爆破器对象，可调用 Start(targets...) 执行爆破
//   - 错误信息，类型不支持时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：对目标执行 ssh 弱口令爆破
// bruter = brute.New("ssh",
//
//	brute.userList("root", "admin"),
//	brute.passList("123456", "password"),
//	brute.concurrent(1),
//
// )~
// res = bruter.Start("127.0.0.1:22")~
//
//	for item = range res {
//	    if item.Ok {
//	        println("found:", item.Username, item.Password)
//	    }
//	}
//
// ```
func _yakitBruterNew(typeStr string, opts ...BruteOpt) (*yakBruter, error) {
	bruter := &yakBruter{
		bruteType:        typeStr,
		concurrentTarget: 256,
		concurrent:       1,
		minDelay:         1,
		maxDelay:         5,
	}
	for _, p := range opts {
		p(bruter)
	}
	if bruter.Ctx == nil {
		bruter.Ctx = context.Background()
	}

	if bruter.coreHandler == nil {
		coreHandler, err := bruteutils.GetBruteFuncByType(bruter.bruteType)
		if err != nil {
			return nil, utils.Errorf("get bruter for [%v] failed: %s", typeStr, err)
		}
		bruter.coreHandler = coreHandler
	}

	if bruter.coreHandler == nil {
		return nil, utils.Errorf("empty bruter for [%s]", typeStr)
	}

	return bruter, nil
}

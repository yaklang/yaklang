package tools

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var BruterExports = map[string]interface{}{
	"GetAvailableBruteTypes": func() []string {
		return bruteutils.GetBuildinAvailableBruteType()
	},
	"GetUsernameListFromBruteType": bruteutils.GetUsernameListFromBruteType,
	"GetPasswordListFromBruteType": bruteutils.GetPasswordListFromBruteType,

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
	"ctx":                YakBruteOpt_ctx,
}

type YakBruter struct {
	debug bool `json:"debug"`

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

	ctx context.Context
}

type YakBruteOpt func(bruter *YakBruter)

func yakBruteOpt_Debug(b bool) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.debug = b
	}
}

func yakBruteOpt_OkToStop(b bool) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.okToStop = b
	}
}

func yakBruteOpt_FinishingThreshold(i int) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.finishingThreshold = i
	}
}

func YakBruteOpt_ctx(ctx context.Context) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.ctx = ctx
	}
}

func yakBruteOpt_ConcurrentTarget(c int) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.concurrentTarget = c
	}
}

func yakBruteOpt_userlist(users ...string) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.userList = users
	}
}

func yakBruteOpt_autoDict() YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.userList = bruteutils.GetUsernameListFromBruteType(bruter.bruteType)
		bruter.passList = bruteutils.GetPasswordListFromBruteType(bruter.bruteType)
	}
}

func yakBruteOpt_passlist(passes ...string) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.passList = passes
	}
}

func yakBruteOpt_minDelay(min int) YakBruteOpt {
	return func(b *YakBruter) {
		b.minDelay = min
	}
}

func yakBruteOpt_concurrent(c int) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.concurrent = c
	}
}

func yakBruteOpt_maxDelay(max int) YakBruteOpt {
	return func(b *YakBruter) {
		b.maxDelay = max
	}
}

func yakBruteOpt_coreHandler(cb func(item *bruteutils.BruteItem) *bruteutils.BruteItemResult) YakBruteOpt {
	return func(bruter *YakBruter) {
		bruter.coreHandler = cb
	}
}

func (y *YakBruter) Start(targets ...string) (chan *bruteutils.BruteItemResult, error) {
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

		err := bruter.StreamBruteContext(y.ctx, y.bruteType, targets, y.userList, y.passList, func(b *bruteutils.BruteItemResult) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			select {
			case ch <- b:
				if !b.Ok {
					return
				}
				// 更新hit_count，+1
				db := consts.GetGormProfileDatabase()
				payloads, err := yakit.QueryPayloadWithoutPaging(db, "", "", b.Username)
				if err != nil {
					log.Errorf("brute failed: %v", err)
					return
				}
				if len(payloads) > 0 {
					var zero int64 = 0
					for _, payload := range payloads {
						if payload.HitCount == nil {
							payload.HitCount = &zero
						}
						zero += 1
						payload.HitCount = &zero
						yakit.UpdatePayload(db, int(payload.ID), payload)
					}
				}

				payloads, err = yakit.QueryPayloadWithoutPaging(db, "", "", b.Password)
				if err != nil {
					log.Errorf("brute failed: %v", err)
					return
				}
				if len(payloads) > 0 {
					var hitCount int64 = 0
					for _, payload := range payloads {
						if payload.HitCount == nil {
							payload.HitCount = &hitCount
						} else {
							hitCount = *payload.HitCount
						}
						hitCount += 1
						payload.HitCount = &hitCount
						yakit.UpdatePayload(db, int(payload.ID), payload)
					}
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

func _yakitBruterNew(typeStr string, opts ...YakBruteOpt) (*YakBruter, error) {
	bruter := &YakBruter{
		bruteType:        typeStr,
		concurrentTarget: 256,
		concurrent:       1,
		minDelay:         1,
		maxDelay:         5,
	}
	for _, p := range opts {
		p(bruter)
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

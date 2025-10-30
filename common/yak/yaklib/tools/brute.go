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

func yakBruteOpt_Debug(b bool) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.debug = b
	}
}

func yakBruteOpt_OkToStop(b bool) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.okToStop = b
	}
}

func yakBruteOpt_FinishingThreshold(i int) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.finishingThreshold = i
	}
}

func yakBruteOpt_ConcurrentTarget(c int) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.concurrentTarget = c
	}
}

func yakBruteOpt_userlist(users ...string) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.userList = users
	}
}

func yakBruteOpt_autoDict() BruteOpt {
	return func(bruter *yakBruter) {
		bruter.userList = bruteutils.GetUsernameListFromBruteType(bruter.bruteType)
		bruter.passList = bruteutils.GetPasswordListFromBruteType(bruter.bruteType)
	}
}

func yakBruteOpt_passlist(passes ...string) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.passList = passes
	}
}

func yakBruteOpt_minDelay(min int) BruteOpt {
	return func(b *yakBruter) {
		b.minDelay = min
	}
}

func yakBruteOpt_concurrent(c int) BruteOpt {
	return func(bruter *yakBruter) {
		bruter.concurrent = c
	}
}

func yakBruteOpt_maxDelay(max int) BruteOpt {
	return func(b *yakBruter) {
		b.maxDelay = max
	}
}

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

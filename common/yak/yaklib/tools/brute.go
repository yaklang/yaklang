package tools

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
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
	"concurrent":         yakBruteOpt_concurrent,
	"minDelay":           yakBruteOpt_minDelay,
	"maxDelay":           yakBruteOpt_maxDelay,
	"bruteHandler":       yakBruteOpt_coreHandler,
	"okToStop":           yakBruteOpt_OkToStop,
	"finishingThreshold": yakBruteOpt_FinishingThreshold,
}

type yakBruter struct {
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
}

type yakBruteOpt func(bruter *yakBruter)

func yakBruteOpt_Debug(b bool) yakBruteOpt {
	return func(bruter *yakBruter) {
		bruter.debug = b
	}
}

func yakBruteOpt_OkToStop(b bool) yakBruteOpt {
	return func(bruter *yakBruter) {
		bruter.okToStop = b
	}
}

func yakBruteOpt_FinishingThreshold(i int) yakBruteOpt {
	return func(bruter *yakBruter) {
		bruter.finishingThreshold = i
	}
}

func yakBruteOpt_ConcurrentTarget(c int) yakBruteOpt {
	return func(bruter *yakBruter) {
		bruter.concurrentTarget = c
	}
}

func yakBruteOpt_userlist(users ...string) yakBruteOpt {
	return func(bruter *yakBruter) {
		bruter.userList = users
	}
}

func yakBruteOpt_passlist(passes ...string) yakBruteOpt {
	return func(bruter *yakBruter) {
		bruter.passList = passes
	}
}

func yakBruteOpt_minDelay(min int) yakBruteOpt {
	return func(b *yakBruter) {
		b.minDelay = min
	}
}

func yakBruteOpt_concurrent(c int) yakBruteOpt {
	return func(bruter *yakBruter) {
		bruter.concurrent = c
	}
}

func yakBruteOpt_maxDelay(max int) yakBruteOpt {
	return func(b *yakBruter) {
		b.maxDelay = max
	}
}

func yakBruteOpt_coreHandler(cb func(item *bruteutils.BruteItem) *bruteutils.BruteItemResult) yakBruteOpt {
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

		if y.userList == nil {
			y.userList = []string{""}
		}
		if y.passList == nil {
			y.passList = []string{""}
		}

		err := bruter.StreamBruteContext(context.Background(), y.bruteType, targets, y.userList, y.passList, func(b *bruteutils.BruteItemResult) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			select {
			case ch <- b:
			}
		})
		if err != nil {
			log.Errorf("build stream context failed: %s", err.Error())
			return
		}
	}()

	return ch, nil
}

func _yakitBruterNew(typeStr string, opts ...yakBruteOpt) (*yakBruter, error) {
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

package bruteutils

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
)

type DefaultServiceAuthInfo struct {
	ServiceName string

	DefaultPorts     string
	DefaultUsernames []string
	DefaultPasswords []string

	UnAuthVerify func(i *BruteItem) *BruteItemResult
	BrutePass    func(i *BruteItem) *BruteItemResult

	CheckedUnAuthTargets map[string]struct{}
	checkedTargetsMutex  sync.RWMutex // 保护CheckedUnAuthTargets的并发访问
}

func (d *DefaultServiceAuthInfo) GetBruteHandler() BruteCallback {
	return func(item *BruteItem) (finalResult *BruteItemResult) {
		defer func() {
			if err := recover(); err != nil {
				result := item.Result()
				result.Ok = false
				result.ExtraInfo = []byte(fmt.Sprintf("brute item failed: %s\nstack:\n%v", err, utils.ErrorStack(err)))
				finalResult = result
			}
		}()

		if strings.Contains(item.Password, "{{params(user)}}") {
			passwords, _ := mutate.QuickMutate(item.Password, consts.GetGormProfileDatabase(), mutate.MutateWithExtraParams(map[string][]string{
				"user": {item.Username},
			}))
			if len(passwords) > 0 {
				item.Password = passwords[0]
			}
		}

		if d.BrutePass == nil && d.UnAuthVerify == nil {
			r := item.Result()
			r.Finished = true
			return r
		}

		// 使用锁保护CheckedUnAuthTargets的初始化
		d.checkedTargetsMutex.Lock()
		if d.CheckedUnAuthTargets == nil {
			d.CheckedUnAuthTargets = make(map[string]struct{})
		}
		d.checkedTargetsMutex.Unlock()

		if d.UnAuthVerify != nil {
			// 使用读锁检查是否已经验证过
			d.checkedTargetsMutex.RLock()
			_, ok := d.CheckedUnAuthTargets[item.Target]
			d.checkedTargetsMutex.RUnlock()

			if !ok {
				result := d.UnAuthVerify(item)

				// 使用写锁标记已验证
				d.checkedTargetsMutex.Lock()
				d.CheckedUnAuthTargets[item.Target] = struct{}{}
				d.checkedTargetsMutex.Unlock()

				if result.Ok {
					result.Username = ""
					result.Password = ""
					return result
				}

				if result.Finished {
					return result
				}
			}
		}

		if d.BrutePass == nil {
			r := item.Result()
			r.Finished = true
			return r
		}
		return d.BrutePass(item)
	}
}

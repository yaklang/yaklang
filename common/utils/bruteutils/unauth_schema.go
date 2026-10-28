package bruteutils

import (
	"fmt"
	"strings"
	"sync"

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

	// Deprecated: unauthenticated checks are tracked by each GetBruteHandler
	// callback so that a later scan can check the same target again.
	CheckedUnAuthTargets map[string]struct{}
}

func (d *DefaultServiceAuthInfo) GetBruteHandler() BruteCallback {
	checkedUnAuthTargets := make(map[string]struct{})
	var checkedTargetsMutex sync.Mutex

	return func(item *BruteItem) (finalResult *BruteItemResult) {
		defer func() {
			if err := recover(); err != nil {
				// panic 栈只进日志，不进入结果
				result := item.Result()
				result.Ok = false
				result.ExtraInfo = []byte(fmt.Sprintf("brute item failed: %v", sanitizePanicMessage(err)))
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

		if d.UnAuthVerify != nil {
			checkedTargetsMutex.Lock()
			_, checked := checkedUnAuthTargets[item.Target]
			if !checked {
				checkedUnAuthTargets[item.Target] = struct{}{}
			}
			checkedTargetsMutex.Unlock()

			if !checked {
				// Probe outside the lock so other targets can run concurrently.
				result := d.UnAuthVerify(item)

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

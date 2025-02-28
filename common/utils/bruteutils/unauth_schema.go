package bruteutils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strings"

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

		if d.UnAuthVerify != nil {
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

		if d.BrutePass == nil {
			r := item.Result()
			r.Finished = true
			return r
		}
		return d.BrutePass(item)
	}
}

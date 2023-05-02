package bruteutils

import (
	"strings"
	"sync"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/mutate"
)

type DefaultServiceAuthInfo struct {
	ServiceName string

	DefaultPorts     string
	DefaultUsernames []string
	DefaultPasswords []string

	UnAuthVerify func(i *BruteItem) *BruteItemResult
	BrutePass    func(i *BruteItem) *BruteItemResult

	// map[string]
	targetAuthChecked *sync.Map
}

func (d *DefaultServiceAuthInfo) GetBruteHandler() BruteCallback {
	return func(item *BruteItem) *BruteItemResult {
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
			if d.targetAuthChecked == nil {
				d.targetAuthChecked = new(sync.Map)
			}

			_, ok := d.targetAuthChecked.Load(item.Target)
			if !ok {
				d.targetAuthChecked.Store(item.Target, nil)
				result := d.UnAuthVerify(item)
				if result.Ok {
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

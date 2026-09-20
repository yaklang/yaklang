package bruteutils

import (
	"context"
	"errors"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/vncprobe"
)

var vncAuth = &DefaultServiceAuthInfo{
	ServiceName:      "vnc",
	DefaultPorts:     "5900",
	DefaultUsernames: append([]string{"vnc"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass:        vncBrutePass,
}

func vncBrutePass(item *BruteItem) *BruteItemResult {
	target := fixToTarget(item.Target, 5900)
	result := item.Result()
	result.OnlyNeedPassword = true

	_, port, _ := utils.ParseStringToHostPort(target)
	if port <= 0 {
		result.Finished = true
		return result
	}

	ctx, cancel := context.WithTimeout(itemCtx(item), defaultTimeout)
	defer cancel()

	err := vncprobe.Probe(ctx, defaultDialer, vncprobe.Options{
		Address:  target,
		Password: item.Password,
		Timeout:  defaultTimeout,
	})
	switch {
	case err == nil:
		result.Ok = true
	case errors.Is(err, vncprobe.ErrAuthFailed):
		// Wrong password: keep other candidates.
	default:
		result.Finished = true
	}
	return result
}

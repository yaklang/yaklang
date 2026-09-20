package bruteutils

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/vncprobe"
)

var vncAuth = &DefaultServiceAuthInfo{
	ServiceName:      "vnc",
	DefaultPorts:     "5900",
	DefaultUsernames: append([]string{"vnc"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     vncUnauthVerify,
	BrutePass:        vncBrutePass,
}

func vncUnauthVerify(item *BruteItem) *BruteItemResult {
	r := vncProbe(item, "", true)
	out := vncApply(item, r)
	if errors.Is(r.Err, vncprobe.ErrNoCompatibleAuth) {
		// None was not offered; password auth may still work.
		out.Finished = false
	}
	return out
}

func vncBrutePass(item *BruteItem) *BruteItemResult {
	return vncApply(item, vncProbe(item, item.Password, false))
}

func vncProbe(item *BruteItem, password string, unauth bool) vncprobe.Result {
	target := fixToTarget(item.Target, 5900)
	item.Target = target
	_, port, _ := utils.ParseStringToHostPort(target)
	if port <= 0 {
		return vncprobe.Result{Err: vncprobe.ErrProtocolMismatch}
	}
	parent := itemCtx(item)
	if err := parent.Err(); err != nil {
		return vncprobe.Result{Err: err}
	}
	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	return vncprobe.Probe(ctx, defaultDialer, vncprobe.Options{
		Address:    target,
		Password:   password,
		Timeout:    defaultTimeout,
		UnauthOnly: unauth,
	})
}

func vncApply(item *BruteItem, r vncprobe.Result) *BruteItemResult {
	out := item.Result()
	out.OnlyNeedPassword = true
	switch {
	case r.Err == nil && r.AuthNone:
		out.Ok = true
		out.Username = ""
		out.Password = ""
		out.ExtraInfo = []byte("auth=none")
	case r.Err == nil:
		out.Ok = true
		out.ExtraInfo = []byte("auth=vnc")
	case r.Locked || errors.Is(r.Err, vncprobe.ErrLocked):
		out.AccountLocked = true
		out.UserEliminated = true
		out.ExtraInfo = []byte("auth=locked")
	case errors.Is(r.Err, vncprobe.ErrAuthFailed):
		// Wrong password: keep other candidates.
	case errors.Is(r.Err, vncprobe.ErrTransient),
		errors.Is(r.Err, context.Canceled),
		errors.Is(r.Err, context.DeadlineExceeded):
		// Handshake timeout / drop after a valid RFB banner: retry.
	case errors.Is(r.Err, vncprobe.ErrProtocolMismatch),
		errors.Is(r.Err, vncprobe.ErrUnsupportedServer),
		errors.Is(r.Err, vncprobe.ErrNoCompatibleAuth):
		out.Finished = true
	case vncUnreachable(r.Err):
		out.Finished = true
	default:
		if vncTimeout(r.Err) {
			break
		}
		out.Finished = true
	}
	return out
}

func vncUnreachable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, n := range []string{
		"connection refused",
		"no route to host",
		"network is unreachable",
		"host is unreachable",
	} {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

func vncTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

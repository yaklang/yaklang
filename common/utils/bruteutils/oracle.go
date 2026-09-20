package bruteutils

import (
	"context"
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/oracleprobe"
)

var oracleServiceNames = []string{"orcl", "xe", "oracle", "orclpdb1", "xepdb1", "freepdb1", "free"}

var oracleAuth = &DefaultServiceAuthInfo{
	ServiceName:      "oracle",
	DefaultPorts:     "1521",
	DefaultUsernames: []string{"sys", "system", "oracle"},
	DefaultPasswords: []string{"sys", "sys123", "system", "password", "123qwe", "123456", "oracle", "oracle001", "oracle.com", "admin123..", "adminroot123", "admin", "root"},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		return i.Result()
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		return oracleBrutePass(i, func(ctx context.Context, o oracleprobe.Options) error {
			return oracleprobe.Probe(ctx, defaultDialer, o)
		})
	},
}

func oracleBrutePass(i *BruteItem, probe func(context.Context, oracleprobe.Options) error) *BruteItemResult {
	i.Target = appendDefaultPort(i.Target, 1521)
	res := i.Result()

	ctx, cancel := context.WithTimeout(itemCtx(i), defaultTimeout)
	defer cancel()
	sawService := false
	allLocked := true
	for _, service := range oracleServiceNames {
		err := probe(ctx, oracleprobe.Options{
			Address: i.Target, Service: service, Username: i.Username,
			Password: i.Password, SysDBA: strings.EqualFold(i.Username, "sys"),
			Timeout: defaultTimeout,
		})
		if err == nil {
			res.Ok, res.Finished = true, true
			return res
		}
		if errors.Is(err, oracleprobe.ErrUnsupportedCredentialEncoding) {
			sawService, allLocked = true, false
			continue
		}
		var oracleErr *oracleprobe.Error
		if errors.As(err, &oracleErr) {
			if oracleErr.ServiceUnknown() {
				continue
			}
			// A rejected credential does not exhaust the target's password
			// candidates. Other accounts may still work when one is locked.
			sawService = true
			allLocked = allLocked && oracleErr.Code == 28000
			continue
		}
		log.Debugf("oracle login probe failed: %v", err)
		res.Finished = true
		return res
	}
	res.Finished = !sawService // unknown service is target-level; wrong password is not
	res.UserEliminated = sawService && allLocked
	return res
}

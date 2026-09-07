package mysql_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/mysql"
)

// 真实服务上的认证边界正反案例（YAK_BRUTE_REAL=1）。
// 前置准备（注意 --default-character-set=utf8mb4，否则 Unicode 凭证会被
// 容器内客户端的默认字符集二次编码，导致"服务端配置问题"假失败）：
//
//	docker exec yak-mysql8 mysql -uroot -pRootPass123! --default-character-set=utf8mb4 -e "
//	  CREATE USER 'expired'@'%' IDENTIFIED BY 'Expired123!'; ALTER USER 'expired'@'%' PASSWORD EXPIRE;
//	  CREATE USER 'locked'@'%' IDENTIFIED BY 'Locked123!'; ALTER USER 'locked'@'%' ACCOUNT LOCK;
//	  CREATE USER 'unicode用户'@'%' IDENTIFIED BY '密码🔐123';
//	  CREATE USER 'longpass'@'%' IDENTIFIED BY 'RealLong<200个a>';"

// 边界语义说明：
//   - expired-correct → AuthSuccess：探针声明 CLIENT_CAN_HANDLE_EXPIRED_PASSWORDS，
//     密码正确即成功（弱口令已命中，服务端是否要求改密不影响判定）
//   - account-locked → AuthFailed：错误号 3118（账户锁定），密码无法验证
func TestMySQLBoundaryCases(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1")
	}
	addr := "127.0.0.1:33306"
	probeOnce := func(user, pass string) core.Result {
		target, _ := core.ParseTarget(addr)
		var p mysql.Prober
		return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
			core.Options{Timeout: 10 * time.Second})
	}

	cases := []struct {
		name string
		user string
		pass string
		want core.Outcome
	}{
		// 密码过期：凭证正确但需修改（CLIENT_CAN_HANDLE_EXPIRED_PASSWORDS 协商）
		{"expired-correct", "expired", "Expired123!", core.OutcomeAuthSuccess},
		{"expired-wrong", "expired", "wrong", core.OutcomeAuthFailed},
		// 账户锁定：即使密码正确也拒绝（错误号 3118）
		{"account-locked-correct", "locked", "Locked123!", core.OutcomeAuthFailed},
		{"account-locked-wrong", "locked", "nope", core.OutcomeAuthFailed},
		// Unicode 用户名+密码（正确凭证）
		{"unicode-correct", "unicode用户", "密码🔐123", core.OutcomeAuthSuccess},
		{"unicode-wrong", "unicode用户", "错的", core.OutcomeAuthFailed},
		// 200 字符长密码（MySQL 允许，正确凭证）
		{"longpass-correct", "longpass", "RealLong" + strings.Repeat("a", 200), core.OutcomeAuthSuccess},
		{"longpass-wrong", "longpass", strings.Repeat("a", 200), core.OutcomeAuthFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := probeOnce(tc.user, tc.pass)
			if res.Outcome != tc.want {
				t.Errorf("want %v got %v (%s)", tc.want, res.Outcome, res.ErrDetail)
			}
		})
	}
}

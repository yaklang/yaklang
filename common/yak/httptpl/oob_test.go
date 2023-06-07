package httptpl

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"
	"time"
)

func RequireOOBAddr(timeout ...float64) {
	t := 3 * time.Second
	if len(timeout) > 0 {
		t = utils.FloatSecondDuration(timeout[0])
	}

	domain, token, err := yakit.NewDNSLogDomainWithContext(utils.TimeoutContext(t))
	if err != nil {
		panic(err)
		return
	}
	_ = domain
	_ = token
}

func TestOOB(t *testing.T) {

}

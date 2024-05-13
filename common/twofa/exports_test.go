package twofa

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"
)

func TestNewTOTPConfig(t *testing.T) {
	id := utils.RandSecret(100)
	config := NewTOTPConfig(id)
	config.GetToptCode()
	spew.Dump(config.GetToptCode())
	config.GetToptUTCCode()
	spew.Dump("TOTP", config.GetToptUTCCode())
	assert.Equal(t, NewTOTPConfig(id).GetToptUTCCode(), config.GetToptUTCCode())
}

func TestNewTOTPServer(t *testing.T) {
	responseId := uuid.New().String()
	target, to := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte(`HTTP/1.1 200 OK
Content-Length: ` + fmt.Sprint(len(responseId)) + `

` + responseId)
	})
	target = utils.HostPort(target, to)

	pPort := utils.GetRandomAvailableTCPPort()

	secret := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		NewOTPServer(secret, pPort, target).ServeContext(ctx)
	}()

	err := utils.WaitConnect("127.0.0.1:"+fmt.Sprint(pPort), 5)
	if err != nil {
		t.Fatal(err)
	}
	code := NewTOTPConfig(secret).GetToptUTCCode()
	rsp, req, err := poc.HTTP(`GET / HTTP/1.1
Host: 127.0.0.1:`+fmt.Sprint(pPort)+`
`, poc.WithAppendHeader("Y-T-Verify-Code", fmt.Sprint(code)))
	if err != nil {
		panic(err)
	}
	_ = req
	fmt.Println(string(rsp))
	if !strings.Contains(string(rsp), responseId) {
		t.Fatal("failed for verify proxy for in totp")
	}
}

func TestComputeCode(t *testing.T) {
	for i := 0; i < 1000; i++ {

		secret := []byte(utils.RandNumberStringBytes(12))
		config := &OTPConfig{
			Secret:     codec.EncodeBase32(secret),
			WindowSize: 3,
			UTC:        true,
		}
		url1 := config.ProvisionURIWithIssuer("v1ll4n@yaklang.io", "testv1ll4n")
		code := config.GetToptUTCCode()
		result, err := config.Authenticate(fmt.Sprint(code))
		if err != nil {
			log.Warnf("invalid code: %v", code)
			panic(err)
		}
		if !result {
			panic("failed")
			return
		}
		fmt.Println(url1)
		_ = config
	}
}

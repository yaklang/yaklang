package twofa

import (
	"context"
	"encoding/base32"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
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
	secret := []byte(utils.RandSecret(100))
	config := &OTPConfig{
		Secret:     base32.StdEncoding.EncodeToString(secret),
		WindowSize: 3,
	}
	url1 := config.ProvisionURIWithIssuer("v1ll4n@yaklang.io", "testv1ll4n")
	result, err := config.Authenticate(fmt.Sprint(config.GetToptCode()))
	if err != nil {
		panic(err)
	}
	if !result {
		panic("failed")
		return
	}
	fmt.Println(url1)
	_ = config
}

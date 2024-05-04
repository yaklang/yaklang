package twofa

import (
	"encoding/base32"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
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

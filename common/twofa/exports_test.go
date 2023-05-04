package twofa

import (
	"encoding/base32"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

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
	println(url1)
	_ = config
}

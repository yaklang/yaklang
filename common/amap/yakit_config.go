package amap

import (
	"encoding/base64"

	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

type YakitAmapConfig struct {
	ApiKey string `app:"name:api_key,verbose:ApiKey,desc:APIKey,required:true,id:1"`
}

func LoadAmapKeywordFromYakit() (string, error) {
	cfg := &YakitAmapConfig{}
	err := consts.GetThirdPartyApplicationConfig("amap", cfg)
	if err != nil {
		return "", err
	}
	return cfg.ApiKey, nil
}

// LoadAmapTOTPHeader generates the X-Memfit-OTP-Auth header value for aibalance proxy authentication.
// It fetches the TOTP secret from the aibalance server and generates a base64-encoded TOTP code.
func LoadAmapTOTPHeader() (string, error) {
	// Use aibalanceclient to generate TOTP code, reusing the same secret cache
	// as the AI gateway client
	totpCode := aibalanceclient.GenerateTOTPCode(func() string {
		// Fetch TOTP secret from the aibalance server
		secret := aibalanceclient.FetchTOTPSecretFromAIBalance()
		if secret == "" {
			log.Warnf("failed to fetch TOTP secret from aibalance server for amap proxy")
		}
		return secret
	})

	if totpCode == "" {
		return "", nil
	}

	// Base64 encode the TOTP code (same format as AI gateway client)
	return base64.StdEncoding.EncodeToString([]byte(totpCode)), nil
}

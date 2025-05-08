package amap

import "github.com/yaklang/yaklang/common/consts"

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

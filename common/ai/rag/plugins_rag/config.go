package plugins_rag

import "github.com/yaklang/yaklang/common/consts"

type EmbeddingEndpointConfig struct {
	BaseURL   string `app:"name:base_url,verbose:BaseURL,desc:BaseURL,required:true,id:1"`
	Model     string `app:"name:model,verbose:Model,desc:Model,required:true,id:2,default:Qwen3-Embedding-0.6B-Q4_K_M"`
	Dimension int    `app:"name:dimension,verbose:Dimension,desc:Dimension,required:true,id:3,default:1024"`
}

func LoadEmbeddingEndpointConfig() (*EmbeddingEndpointConfig, error) {
	cfg := &EmbeddingEndpointConfig{}
	err := consts.GetThirdPartyApplicationConfig("embedding_endpoint", cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

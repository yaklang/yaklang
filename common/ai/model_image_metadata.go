package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var modelImageMetadata = utils.NewCacheExWithKey[[32]byte, map[string]bool](
	utils.WithCacheCapacity(128), utils.WithCacheTTL(5*time.Minute),
)

func supportsModelMetadataDiscovery(provider string) bool {
	// These gateways discover models with a GET to their model-list API.
	switch provider {
	case "deepseek", "openrouter", "custom", "openai", "aibalance":
		return true
	default:
		return false
	}
}

func modelMetadataKey(cfg *aispec.AIConfig) [32]byte {
	connection, _ := json.Marshal(struct {
		Type, Domain, BaseURL, Endpoint, APIType, APIKey, Proxy, Host string
		EnableEndpoint, NoHTTPS                                       bool
		Port                                                          int
		Headers                                                       map[string]string
	}{
		Type: cfg.Type, Domain: cfg.Domain, BaseURL: cfg.BaseURL,
		Endpoint: cfg.Endpoint, APIType: cfg.APIType, APIKey: cfg.APIKey,
		Proxy: cfg.Proxy, Host: cfg.Host, EnableEndpoint: cfg.EnableEndpoint,
		NoHTTPS: cfg.NoHttps, Port: cfg.Port, Headers: aispec.ExtraHeadersToMap(cfg.Headers),
	})
	return sha256.Sum256(connection)
}

func imageModalities(models []*aispec.ModelMeta) map[string]bool {
	support := make(map[string]bool)
	for _, model := range models {
		if model == nil || model.Id == "" {
			continue
		}
		modalities := model.InputModalities
		if len(modalities) == 0 && model.Architecture != nil {
			modalities = model.Architecture.InputModalities
		}
		if len(modalities) != 0 {
			support[model.Id] = slices.Contains(modalities, "image")
		}
	}
	return support
}

// DiscoverModelImageInput reads the configured provider's metadata. A nil
// result leaves the capability unknown. Tool calling with images requires
// separate evidence, even when the provider declares both features.
func DiscoverModelImageInput(ctx context.Context, model *ypb.AIModelConfig) *bool {
	if model.GetModelName() == "" || !supportsModelMetadataDiscovery(model.GetProvider().GetType()) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	opts := append(aispec.BuildOptionsFromConfig(model), aispec.WithContext(ctx))
	cfg := aispec.NewDefaultAIConfig(opts...)
	key := modelMetadataKey(cfg)
	capabilities, err := modelImageMetadata.GetOrLoad(key, func() (map[string]bool, error) {
		client := GetAI(cfg.Type, opts...)
		if client == nil {
			return nil, nil
		}
		models, err := client.GetModelList()
		if err != nil {
			// Suppress repeated lookups after a transient failure. An explicit
			// model-list refresh can replace the cached result.
			if ctx.Err() != context.Canceled {
				modelImageMetadata.SetWithTTL(key, nil, 30*time.Second)
			}
			return nil, err
		}
		return imageModalities(models), nil
	})
	supported, known := capabilities[model.GetModelName()]
	if err != nil || !known {
		return nil
	}
	return &supported
}

package aicommon

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// WithLiteForgeSpeedFirst configures the LiteForge to prefer speed-priority (lightweight) AI model.
//
// Behavior:
//  1. If AI callbacks are already set (e.g., via WithAICallback), it promotes the existing
//     SpeedPriorityAICallback to be used as the primary callback.
//  2. If no callbacks are set yet and tiered AI config is enabled, it loads the lightweight model
//     from the tiered configuration and uses it for all callbacks.
//  3. Otherwise, it's a no-op and the default callback will be used.
//
// Example:
//
//	result, err := aicommon.InvokeLiteForge(prompt, aicommon.WithLiteForgeSpeedFirst())
func WithLiteForgeSpeedFirst() ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}

		// If callbacks are already set (e.g., by WithAICallback or parent config),
		// promote SpeedPriorityAICallback to be the primary callback
		c.m.Lock()
		if c.SpeedPriorityAICallback != nil {
			c.QualityPriorityAICallback = c.SpeedPriorityAICallback
			c.m.Unlock()
			return nil
		}
		c.m.Unlock()

		// No callbacks set yet, try to load lightweight model from tiered AI config
		if consts.IsTieredAIModelConfigEnabled() {
			lightweightConfigs := consts.GetLightweightAIConfigs()
			if len(lightweightConfigs) > 0 {
				lightweightCB, err := loadCallbackFromThirdPartyConfig(lightweightConfigs[0])
				if err == nil {
					lightweightCB = c.wrapper(lightweightCB)
					c.m.Lock()
					c.QualityPriorityAICallback = lightweightCB
					c.SpeedPriorityAICallback = lightweightCB
					c.m.Unlock()
					log.Debugf("LiteForge speed-first: configured with lightweight model from tiered config")
					return nil
				}
				log.Warnf("LiteForge speed-first: failed to load lightweight model callback: %v", err)
			}
		}

		return nil
	}
}

// WithLiteForgeQualityFirst configures the LiteForge to prefer quality-priority (intelligent) AI model.
//
// Behavior:
//  1. If AI callbacks are already set (e.g., via WithAICallback), it promotes the existing
//     QualityPriorityAICallback to be used for all callbacks (including speed).
//  2. If no callbacks are set yet and tiered AI config is enabled, it loads the intelligent model
//     from the tiered configuration and uses it for all callbacks.
//  3. Otherwise, it's a no-op and the default callback will be used.
//
// Example:
//
//	result, err := aicommon.InvokeLiteForge(prompt, aicommon.WithLiteForgeQualityFirst())
func WithLiteForgeQualityFirst() ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}

		// If callbacks are already set (e.g., by WithAICallback or parent config),
		// promote QualityPriorityAICallback to be used for all callbacks
		c.m.Lock()
		if c.QualityPriorityAICallback != nil {
			c.SpeedPriorityAICallback = c.QualityPriorityAICallback
			c.m.Unlock()
			return nil
		}
		c.m.Unlock()

		// No callbacks set yet, try to load intelligent model from tiered AI config
		if consts.IsTieredAIModelConfigEnabled() {
			intelligentConfigs := consts.GetIntelligentAIConfigs()
			if len(intelligentConfigs) > 0 {
				intelligentCB, err := loadCallbackFromThirdPartyConfig(intelligentConfigs[0])
				if err == nil {
					intelligentCB = c.wrapper(intelligentCB)
					c.m.Lock()
					c.QualityPriorityAICallback = intelligentCB
					c.SpeedPriorityAICallback = intelligentCB
					c.m.Unlock()
					log.Debugf("LiteForge quality-first: configured with intelligent model from tiered config")
					return nil
				}
				log.Warnf("LiteForge quality-first: failed to load intelligent model callback: %v", err)
			}
		}

		return nil
	}
}

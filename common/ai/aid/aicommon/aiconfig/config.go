package aiconfig

import (
	"github.com/yaklang/yaklang/common/consts"
)

// ModelTier represents the tier/category of AI models
type ModelTier string

const (
	// TierIntelligent represents high-intelligence models for complex tasks
	TierIntelligent ModelTier = "intelligent"
	// TierLightweight represents lightweight models for simple and fast tasks
	TierLightweight ModelTier = "lightweight"
	// TierVision represents vision models for image understanding tasks
	TierVision ModelTier = "vision"
)

// RoutingPolicy is an alias to consts.RoutingPolicy for convenience
type RoutingPolicy = consts.RoutingPolicy

// Re-export policy constants for convenience
const (
	PolicyAuto        = consts.PolicyAuto
	PolicyPerformance = consts.PolicyPerformance
	PolicyCost        = consts.PolicyCost
	PolicyBalance     = consts.PolicyBalance
)

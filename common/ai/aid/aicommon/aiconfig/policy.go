package aiconfig

import (
	"github.com/yaklang/yaklang/common/log"
)

// SelectTierByPolicy determines which model tier to use based on the routing policy and context
func SelectTierByPolicy(policy RoutingPolicy, isComplex bool) ModelTier {
	switch policy {
	case PolicyPerformance:
		// Always use intelligent model for performance
		return TierIntelligent
	case PolicyCost:
		// Always use lightweight model for cost efficiency
		return TierLightweight
	case PolicyBalance:
		// Use lightweight by default, intelligent for complex tasks
		if isComplex {
			return TierIntelligent
		}
		return TierLightweight
	case PolicyAuto:
		// Auto mode: same as balance for now
		if isComplex {
			return TierIntelligent
		}
		return TierLightweight
	default:
		log.Warnf("Unknown routing policy: %s, defaulting to lightweight", policy)
		return TierLightweight
	}
}

// IsComplexTask attempts to determine if a task/prompt is complex
// This is a heuristic function that can be improved over time
func IsComplexTask(prompt string) bool {
	// Simple heuristic: longer prompts or prompts with certain keywords are considered complex
	if len(prompt) > 500 {
		return true
	}

	// Check for complexity indicators
	complexityKeywords := []string{
		"分析", "analyze", "analysis",
		"推理", "reason", "reasoning",
		"代码", "code", "programming",
		"复杂", "complex", "complicated",
		"详细", "detailed", "comprehensive",
		"解释", "explain", "explanation",
	}

	for _, keyword := range complexityKeywords {
		if containsIgnoreCase(prompt, keyword) {
			return true
		}
	}

	return false
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	// Simple implementation - for better performance, use strings.ToLower
	sLower := toLower(s)
	substrLower := toLower(substr)

	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

// toLower converts a string to lowercase (simple ASCII-only implementation)
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// AutoSelectTier automatically selects the appropriate tier based on the prompt
func AutoSelectTier(prompt string) ModelTier {
	policy := GetCurrentPolicy()
	isComplex := IsComplexTask(prompt)
	return SelectTierByPolicy(policy, isComplex)
}

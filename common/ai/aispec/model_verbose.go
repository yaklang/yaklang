package aispec

import "strings"

var knownModelNames = map[string]string{
	"gpt-3.5-turbo":    "GPT-3.5-Turbo",
	"gpt-4":            "GPT-4",
	"gpt-4-turbo":      "GPT-4-Turbo",
	"gpt-4o":           "GPT-4o",
	"gpt-4o-mini":      "GPT-4o-Mini",
	"gpt-4.1":          "GPT-4.1",
	"gpt-4.1-mini":     "GPT-4.1-Mini",
	"gpt-4.1-nano":     "GPT-4.1-Nano",
	"o1":               "O1",
	"o1-mini":          "O1-Mini",
	"o1-preview":       "O1-Preview",
	"o3":               "O3",
	"o3-mini":          "O3-Mini",
	"o4-mini":          "O4-Mini",
	"deepseek-v3":      "DeepSeek-V3",
	"deepseek-r1":      "DeepSeek-R1",
	"deepseek-chat":    "DeepSeek-Chat",
	"deepseek-coder":   "DeepSeek-Coder",
	"qwen-max":         "Qwen-Max",
	"qwen-plus":        "Qwen-Plus",
	"qwen-turbo":       "Qwen-Turbo",
	"qwen-long":        "Qwen-Long",
	"qwen-next":        "Qwen-Next",
	"qwen2.5-coder":    "Qwen2.5-Coder",
	"qwen3-235b-a22b":  "Qwen3-235B-A22B",
	"glm-4":            "GLM-4",
	"glm-4-flash":      "GLM-4-Flash",
	"glm-4-plus":       "GLM-4-Plus",
	"claude-3-opus":    "Claude-3-Opus",
	"claude-3-sonnet":  "Claude-3-Sonnet",
	"claude-3-haiku":   "Claude-3-Haiku",
	"claude-3.5-sonnet": "Claude-3.5-Sonnet",
	"claude-4-sonnet":  "Claude-4-Sonnet",
	"claude-4-opus":    "Claude-4-Opus",
	"gemini-pro":       "Gemini-Pro",
	"gemini-1.5-pro":   "Gemini-1.5-Pro",
	"gemini-2.0-flash": "Gemini-2.0-Flash",
	"gemini-2.5-pro":   "Gemini-2.5-Pro",
	"gemini-2.5-flash": "Gemini-2.5-Flash",
	"moonshot-v1-8k":   "Moonshot-V1-8K",
	"moonshot-v1-32k":  "Moonshot-V1-32K",
	"moonshot-v1-128k": "Moonshot-V1-128K",
	"yi-large":         "Yi-Large",
	"yi-medium":        "Yi-Medium",
	"doubao-pro":       "Doubao-Pro",
	"doubao-lite":      "Doubao-Lite",
	"ernie-4.0":        "ERNIE-4.0",
	"ernie-3.5":        "ERNIE-3.5",
}

// ModelVerboseName returns a human-friendly display name for a model identifier.
// It strips the "memfit-" prefix and "-free" suffix, then applies known casing corrections.
func ModelVerboseName(model string) string {
	if model == "" {
		return ""
	}

	cleaned := model
	cleaned = strings.TrimPrefix(cleaned, "memfit-")
	cleaned = strings.TrimSuffix(cleaned, "-free")

	if verbose, ok := knownModelNames[strings.ToLower(cleaned)]; ok {
		return verbose
	}

	return cleaned
}

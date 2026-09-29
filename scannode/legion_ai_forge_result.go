package scannode

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"strings"
)

// Only the Legion adapter owns platform recovery policy. Retry final generation
// in the same context; never rerun the Coordinator or deliver an intermediate
// result to the Blueprint result handler.
func legionForgeResultGenerator(generate func(*aid.Coordinator, string) (string, error)) func(*aid.Coordinator, string) (string, error) {
	return func(cod *aid.Coordinator, prompt string) (string, error) {
		for attempt := 0; attempt < 2; attempt++ {
			result, err := generate(cod, prompt)
			if err != nil {
				return result, err
			}
			if strings.TrimSpace(result) != "" {
				return result, nil
			}
			prompt += "\n\n上一轮未生成可交付的正文。请在最终输出通道直接给出完整结果，并严格遵守原始指令要求的输出格式；不要只在思考内容中写结果。"
		}
		return "", fmt.Errorf("forge result model returned empty final output after retry")
	}
}

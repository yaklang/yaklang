package loopinfra

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// FocusModeUserInputAugmenter augments user input before a focus mode loop runs.
// When PE or UI creates a subtask with only a short goal (e.g. "根据上述代码生成规则")
// but not the actual code, the augmenter can extract full input from timeline or other context.
// Returns augmented input; if no augmentation needed, returns the original userInput.
type FocusModeUserInputAugmenter func(cfg aicommon.AICallerConfigIf, userInput string) string

var focusModeAugmenters sync.Map // identifier -> FocusModeUserInputAugmenter

// RegisterFocusModeUserInputAugmenter registers an augmenter for a focus mode.
// Called by loop packages (e.g. loop_syntaxflow_rule) to provide loop-specific augmentation.
func RegisterFocusModeUserInputAugmenter(identifier string, aug FocusModeUserInputAugmenter) {
	focusModeAugmenters.Store(identifier, aug)
}

func getFocusModeUserInputAugmenter(identifier string) FocusModeUserInputAugmenter {
	v, ok := focusModeAugmenters.Load(identifier)
	if !ok {
		return nil
	}
	aug, _ := v.(FocusModeUserInputAugmenter)
	return aug
}

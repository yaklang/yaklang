package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
)

func (t *AiTask) withMidtermFork(f *aimem.MidtermMemoryFork) func() {
	prev := t.midtermFork
	t.midtermFork = f
	return func() {
		t.midtermFork = prev
	}
}

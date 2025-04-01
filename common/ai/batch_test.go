package ai

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
)

func TestBatchChatter(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	bc := NewBatchChatter()
	apikey, err := os.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "siliconflow.txt"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(apikey))
	bc.SetDebug(true)
	apiKeys := utils.ParseStringToLines(string(apikey))
	bc.SetCallback(func(typeName string, modelName string, isReason bool, reader io.Reader) {
		fmt.Println(typeName, modelName, isReason)
		io.Copy(io.Discard, reader)
	})
	bc.AddChatClientWithManyAPIKeys(
		"siliconflow",
		apiKeys,
		"Qwen/QwQ-32B",
		//"deepseek-ai/DeepSeek-V3",
	)
	result, err := bc.Chat(`
siliconflow Qwen/QwQ-32B true
[INFO] 2025-04-01 16:52:42 [batch:75] [AI Reason] siliconflow - Qwen/QwQ-32B
Okay, the user said "Hello, world!" that's the classic first program. I should respond politely. Maybe say hello back and ask how I can help. Keep it friendly and open-ended.

Hmm, I should make sure not to overcomplicate it. Just a simple greeting and offer assistance. Yeah, that should work.
[INFO] 2025-04-01 16:52:43 [batch:77] --- End of AI Reason ---
siliconflow Qwen/QwQ-32B false
[INFO] 2025-04-01 16:52:43 [batch:75] [AI Response] siliconflow - Qwen/QwQ-32B


Hello! It's great to see you. How can I assist you today? Feel free to ask me any questions or let me know if you need help with anything specific! 😊[INFO] 2025-04-01 16:52:44 [batch:77] --- End of AI Response ---
[INFO] 2025-04-01 16:52:44 [exec:819] close reader and writer
`)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)

	result, err = bc.Chat(`
siliconflow Qwen/QwQ-32B true
[INFO] 2025-04-01 16:52:42 [batch:75] [AI Reason] siliconflow - Qwen/QwQ-32B
Okay, the user said "Hello, world!" that's the classic first program. I should respond politely. Maybe say hello back and ask how I can help. Keep it friendly and open-ended.

Hmm, I should make sure not to overcomplicate it. Just a simple greeting and offer assistance. Yeah, that should work.
[INFO] 2025-04-01 16:52:43 [batch:77] --- End of AI Reason ---
siliconflow Qwen/QwQ-32B false
[INFO] 2025-04-01 16:52:43 [batch:75] [AI Response] siliconflow - Qwen/QwQ-32B


Hello! It's great to see you. How can I assist you today? Feel free to ask me any questions or let me know if you need help with anything specific! 😊[INFO] 2025-04-01 16:52:44 [batch:77] --- End of AI Response ---
[INFO] 2025-04-01 16:52:44 [exec:819] close reader and writer
`)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}

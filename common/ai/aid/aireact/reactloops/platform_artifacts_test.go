package reactloops

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPlatformLoopArtifactsDoNotWriteWritableDirectory(t *testing.T) {
	for _, restricted := range []bool{true, false} {
		name := "ordinary"
		if restricted {
			name = "platform"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			// Establish that writes are permitted; the restricted result cannot be
			// explained by account permissions or an inaccessible directory.
			probe := filepath.Join(dir, "write-probe")
			if err := os.WriteFile(probe, []byte("writable"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(probe); err != nil {
				t.Fatal(err)
			}
			cfg := &aicommon.Config{Workdir: dir}
			if restricted {
				if err := aicommon.WithEphemeralStorage()(cfg); err != nil {
					t.Fatal(err)
				}
			}
			emitter := aicommon.NewEmitter("platform-artifacts", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) { return e, nil })
			task := aicommon.NewStatefulTaskBase("real-turn", "test", context.Background(), emitter)
			loop := &ReActLoop{actionHistoryMutex: new(sync.Mutex), config: cfg, loopName: "default", emitter: emitter, vars: omap.NewEmptyOrderedMap[string, any]()}
			loop.SetCurrentTask(task)
			loopDir := loop.ensureLoopDirectory(task)
			loop.savePromptToFile(task, 1, "private prompt")
			action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"answer"}`, "directly_answer")
			if err != nil {
				t.Fatal(err)
			}
			loop.emitActionExecutionRecord(task, action, 1, "private prompt")
			spill, _ := SaveSpillContent(loop, "private", "tool observation")
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if restricted {
				if len(files) != 0 || loopDir != "" || spill != "" {
					t.Fatalf("platform persisted artifacts: %v %q %q", files, loopDir, spill)
				}
				// Even stale metadata cannot bypass the content-directory gate.
				loop.Set("task_directory", dir)
				loop.Set("loop_name_prefix", "loop_default")
				if got := loop.GetLoopContentDir("prompts"); got != "" {
					t.Fatalf("stale metadata bypass: %s", got)
				}
			} else {
				if len(files) == 0 || loopDir == "" || spill == "" {
					t.Fatal("ordinary artifact path stopped writing")
				}
				data, err := os.ReadFile(spill)
				if err != nil || string(data) != "tool observation" {
					t.Fatalf("ordinary spill changed: %q %v", data, err)
				}
			}
		})
	}
}

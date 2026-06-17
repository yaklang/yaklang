package loopinfra

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const deferredDiskWriteNote = "disk write deferred for frontend review"

func (f *SingleFileModificationSuiteFactory) persistLoopFileContent(
	runtime aicommon.AIInvokeRuntime,
	filename, content, successEntry, failEntry, successMsg string,
) error {
	if f.deferDiskWrite {
		runtime.AddToTimeline(successEntry, successMsg+"; "+deferredDiskWriteNote)
		return nil
	}

	if dir := filepath.Dir(filename); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			runtime.AddToTimeline(failEntry, fmt.Sprintf("FAILED to create parent dir for file: %s, error: %s", filename, err.Error()))
			return err
		}
	}
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		runtime.AddToTimeline(failEntry, fmt.Sprintf("FAILED to write file: %s, error: %s", filename, err.Error()))
		return err
	}
	runtime.AddToTimeline(successEntry, successMsg)
	return nil
}

func (f *SingleFileModificationSuiteFactory) replaceLoopFileContent(
	runtime aicommon.AIInvokeRuntime,
	filename, content, successEntry, failEntry, successMsg string,
) error {
	if f.deferDiskWrite {
		runtime.AddToTimeline(successEntry, successMsg+"; "+deferredDiskWriteNote)
		return nil
	}

	os.RemoveAll(filename)
	return f.persistLoopFileContent(runtime, filename, content, successEntry, failEntry, successMsg)
}

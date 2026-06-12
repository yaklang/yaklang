package loopinfra

import (
	"fmt"
	"os"

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

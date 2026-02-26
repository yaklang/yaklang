package loop_smart_qa

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func appendSearchResults(loop *reactloops.ReActLoop, newContent string) {
	old := loop.Get("search_results_summary")
	if old == "" {
		loop.Set("search_results_summary", newContent)
	} else {
		loop.Set("search_results_summary", old+"\n\n"+newContent)
	}
}

func appendSearchHistory(loop *reactloops.ReActLoop, entry string) {
	old := loop.Get("search_history")
	timestamp := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s", timestamp, entry)
	if old == "" {
		loop.Set("search_history", line)
	} else {
		loop.Set("search_history", old+"\n"+line)
	}
}

func appendMemoryResults(loop *reactloops.ReActLoop, content string) {
	old := loop.Get("memory_results")
	if old == "" {
		loop.Set("memory_results", content)
	} else {
		loop.Set("memory_results", old+"\n\n"+content)
	}
}

func appendFileResults(loop *reactloops.ReActLoop, content string) {
	old := loop.Get("file_results")
	if old == "" {
		loop.Set("file_results", content)
	} else {
		loop.Set("file_results", old+"\n\n"+content)
	}
}

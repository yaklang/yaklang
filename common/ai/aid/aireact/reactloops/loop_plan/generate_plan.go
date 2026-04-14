package loop_plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func isNestedSubtask(parents []string) bool {
	for _, p := range parents {
		if p == "sub_subtasks" {
			return true
		}
	}
	return false
}

func planTasksStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	var taskCount atomic.Int32
	var subTaskCount atomic.Int32

	type orderCtx struct {
		waitCh <-chan struct{}
		doneCh chan struct{}
	}
	var ctxMap sync.Map

	initCh := make(chan struct{})
	close(initCh)
	prevDone := (<-chan struct{})(initCh)

	wg := new(sync.WaitGroup)

	syncCallback := func(key string, reader io.Reader, parents []string) {
		myDone := make(chan struct{})
		ctxMap.Store(reader, &orderCtx{waitCh: prevDone, doneCh: myDone})
		prevDone = myDone
	}

	handler := func(key string, reader io.Reader, parents []string) {
		v, _ := ctxMap.LoadAndDelete(reader)
		oc := v.(*orderCtx)

		wg.Add(1)
		defer wg.Done()
		defer close(oc.doneCh)

		nested := isNestedSubtask(parents)
		indent := ""
		if nested {
			indent = "    "
		}

		reader = utils.JSONStringReader(reader)
		var buf bytes.Buffer
		switch key {
		case "subtask_name":
			if nested {
				if subTaskCount.Add(1) > 1 || taskCount.Load() > 0 {
					buf.WriteString("\n")
				}
				buf.WriteString(indent)
			} else {
				subTaskCount.Store(0)
				if taskCount.Add(1) > 1 {
					buf.WriteString("\n\n")
				}
			}
			buf.WriteString("- [ ] ")
			io.Copy(&buf, reader)
		case "subtask_goal":
			buf.WriteString(": ")
			io.Copy(&buf, reader)
		case "subtask_identifier":
			buf.WriteString(" #")
			io.Copy(&buf, reader)
		case "depends_on":
			raw, _ := io.ReadAll(reader)
			trimmed := strings.TrimSpace(string(raw))
			if trimmed != "" && trimmed != "[]" {
				var deps []string
				if json.Unmarshal([]byte(trimmed), &deps) == nil && len(deps) > 0 {
					buf.WriteString(fmt.Sprintf(" (depends: %s)", strings.Join(deps, ", ")))
				}
			}
		}

		<-oc.waitCh
		buf.WriteTo(emitWriter)
	}

	err := jsonextractor.ExtractStructuredJSONFromStream(fieldReader,
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("subtask_name", handler, syncCallback),
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("subtask_identifier", handler, syncCallback),
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("subtask_goal", handler, syncCallback),
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("depends_on", handler, syncCallback),
		jsonextractor.WithStreamErrorCallback(func(err error) {
			log.Errorf("plan tasks stream parse error: %v", err)
		}),
	)
	if err != nil {
		log.Errorf("plan tasks stream handler error: %v", err)
	}

	wg.Wait()
}

var finishExploration = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	_ = r
	return reactloops.WithRegisterLoopAction(
		"finish_exploration",
		"Signal that information gathering is complete. The system will automatically generate a guidance document and execution plan based on the collected FACTS and evidence.",
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			log.Infof("plan loop: finish_exploration called, exiting exploration phase")
			op.Exit()
		},
	)
}

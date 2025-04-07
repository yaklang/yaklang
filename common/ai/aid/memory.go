package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
	"text/template"
)

type Memory struct {
	Query              string
	CurrentTask        *aiTask
	RootTask           *aiTask
	PlanHistory        []*PlanRecord
	toolCallResults    []*aitool.ToolResult
	InteractiveHistory *omap.OrderedMap[string, *InteractiveEventRecord]
	MetaInfo           map[string]string
}

type PlanRecord struct {
	PlanRequest  *planRequest
	PlanResponse *planResponse
}

type InteractiveEventRecord struct {
	InteractiveEvent *Event
	UserInput        aitool.InvokeParams
}

func NewMemory() *Memory {
	return &Memory{
		PlanHistory:        make([]*PlanRecord, 0),
		MetaInfo:           make(map[string]string),
		InteractiveHistory: omap.NewOrderedMap[string, *InteractiveEventRecord](make(map[string]*InteractiveEventRecord)),
		toolCallResults:    make([]*aitool.ToolResult, 0),
	}
}

func (m *Memory) StoreQuery(query string) {
	m.Query = query
}

func (m *Memory) Progress() string {
	return m.RootTask.Progress()
}

func (m *Memory) SetCurrentTask(task *aiTask) {
	m.CurrentTask = task
}

func (m *Memory) RootPlan() string {
	return m.RootTask.Progress()
}

func (m *Memory) CurrentTaskPlan() string {
	return m.CurrentTask.Progress()
}

func (m *Memory) LastPlanResponse() string {
	if len(m.PlanHistory) == 0 {
		return ""
	}
	return m.PlanHistory[len(m.PlanHistory)-1].PlanResponse.RootTask.Progress()
}

func (m *Memory) PushToolCallResults(t ...*aitool.ToolResult) {
	m.toolCallResults = append(m.toolCallResults, t...)
}

func (m *Memory) PromptForToolCallResultsForLastN(n int) string {
	if len(m.toolCallResults) == 0 {
		return ""
	}

	var result = m.toolCallResults
	if len(result) > n {
		result = result[len(result)-n:]
	}
	templatedata := map[string]interface{}{
		"ToolCallResults": result,
	}
	temp, err := template.New("tool-result-history").Parse(__prompt_ToolResultHistoryPromptTemplate)
	if err != nil {
		log.Errorf("error parsing tool result history template: %v", err)
		return ""
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templatedata)
	if err != nil {
		log.Errorf("error executing tool result history template: %v", err)
		return ""
	}
	return promptBuilder.String()
}

func (m *Memory) PromptForToolCallResultsForLast5() string {
	return m.PromptForToolCallResultsForLastN(5)
}

func (m *Memory) PromptForToolCallResultsForLast10() string {
	return m.PromptForToolCallResultsForLastN(10)
}

func (m *Memory) PromptForToolCallResultsForLast20() string {
	return m.PromptForToolCallResultsForLastN(20)
}

func (m *Memory) StoreInteractiveEvent(eventID string, e *Event) {
	m.InteractiveHistory.Set(eventID, &InteractiveEventRecord{
		InteractiveEvent: e,
	})
}

func (m *Memory) StoreInteractiveUserInput(eventID string, invoke aitool.InvokeParams) {
	record, ok := m.InteractiveHistory.Get(eventID)
	if !ok {
		log.Errorf("error getting review record for event ID %s", eventID)
		return
	}
	record.UserInput = invoke
}

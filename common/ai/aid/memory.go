package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	osRuntime "runtime"
	"strings"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type PlanRecord struct { // todo
	PlanRequest  *planRequest
	PlanResponse *PlanResponse
}

type InteractiveEventRecord struct {
	InteractiveEvent *Event
	UserInput        aitool.InvokeParams
}

type PersistentDataRecord struct {
	Variable bool
	Value    string
}

type Memory struct {
	// user first input
	Query string

	// persistent data
	PersistentData *omap.OrderedMap[string, *PersistentDataRecord]

	// task info
	CurrentTask *aiTask
	RootTask    *aiTask

	// todo
	PlanHistory []*PlanRecord

	// tools list
	DisableTools          bool
	Tools                 func() []*aitool.Tool
	toolsKeywordsCallback func() []string

	// tool call results
	//toolCallResults []*aitool.ToolResult

	// interactive history
	InteractiveHistory *omap.OrderedMap[string, *InteractiveEventRecord]

	timeline *memoryTimeline // timeline with tool call results, will reduce the memory size
}

func (m *Memory) CopyReducibleMemory() *Memory {
	mem := &Memory{
		PersistentData:        m.PersistentData.Copy(),
		DisableTools:          m.DisableTools,
		Tools:                 m.Tools,
		toolsKeywordsCallback: m.toolsKeywordsCallback,
		InteractiveHistory:    m.InteractiveHistory.Copy(),

		// task && plan is not reducible, remove it
		CurrentTask: nil,
		RootTask:    nil,
		PlanHistory: nil,
	}
	mem.timeline = m.timeline.CopyReducibleTimelineWithMemory(mem)
	return m
}

func GetDefaultMemory() *Memory {
	return &Memory{
		PlanHistory:        make([]*PlanRecord, 0),
		PersistentData:     omap.NewOrderedMap[string, *PersistentDataRecord](make(map[string]*PersistentDataRecord)),
		InteractiveHistory: omap.NewOrderedMap[string, *InteractiveEventRecord](make(map[string]*InteractiveEventRecord)),
		Tools: func() []*aitool.Tool {
			return make([]*aitool.Tool, 0)
		},
		timeline: newMemoryTimeline(10, nil),
	}
}

func (m *Memory) BindCoordinator(c *Coordinator) {
	config := c.config
	m.StoreQuery(c.userInput)
	m.StoreTools(func() []*aitool.Tool {
		alltools, err := config.aiToolManager.GetEnableTools()
		if err != nil {
			log.Errorf("coordinator: get all tools failed: %v", err)
			return nil
		}
		return alltools
	})
	m.StoreToolsKeywords(func() []string {
		return config.keywords
	})
	m.PushPersistentData(config.persistentMemory...)
	m.timeline.BindConfig(config)
}

// user data memory api, user or ai can set and get

func (m *Memory) PushPersistentData(values ...string) {
	for _, value := range values {
		m.PersistentData.Set(codec.Sha1(value), &PersistentDataRecord{
			Variable: false,
			Value:    value,
		})
	}
}

func (m *Memory) SetPersistentData(key string, value string) {
	m.PersistentData.Set(key, &PersistentDataRecord{
		Variable: true,
		Value:    value,
	})
}

func (m *Memory) GetPersistentData(key string) (string, bool) {
	res, ok := m.PersistentData.Get(key)
	if ok {
		return res.Value, ok
	}
	return "", ok
}

func (m *Memory) DeletePersistentData(key string) {
	m.PersistentData.Delete(key)
}

// constants info memory api
func (m *Memory) Now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func (m *Memory) OS() string {
	return osRuntime.GOOS
}

func (m *Memory) Arch() string {
	return osRuntime.GOARCH
}

func (m *Memory) Schema() map[string]string {
	var toolNames []string
	for _, tool := range m.Tools() {
		toolNames = append(toolNames, tool.Name)
	}
	return planJSONSchema(toolNames)
}

// set tools list
func (m *Memory) StoreTools(toolList func() []*aitool.Tool) {
	m.Tools = toolList
}

func (m *Memory) ClearRuntimeConfig() {
	m.timeline.ai = nil
	m.timeline.config = nil
}

// set tools list
func (m *Memory) StoreToolsKeywords(keywords func() []string) {
	m.toolsKeywordsCallback = keywords
}

func (m *Memory) ToolsKeywords() string {
	return strings.Join(m.toolsKeywordsCallback(), ", ")
}

// user first input
func (m *Memory) StoreQuery(query string) {
	m.Query = query
}

// task info memory
func (m *Memory) StoreRootTask(t *aiTask) {
	m.RootTask = t
}

func (m *Memory) Progress() string {
	return m.RootTask.Progress()
}

func (m *Memory) StoreCurrentTask(task *aiTask) {
	m.CurrentTask = task
}

// interactive history memory
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

func (m *Memory) GetInteractiveEventLast() (string, *InteractiveEventRecord, bool) {
	return m.InteractiveHistory.Last()
}

func (m *Memory) GetInteractiveEvent(eventID string) (*InteractiveEventRecord, bool) {
	return m.InteractiveHistory.Get(eventID)
}

// tool results memory
func (m *Memory) PushToolCallResults(t *aitool.ToolResult) {
	m.timeline.PushToolResult(t)
}

func (m *Memory) Timeline() string {
	return m.timeline.Dump()
}

func (m *Memory) TimelineWithout(n ...any) string {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error creating sub timeline: %v", r)
			fmt.Println(utils.ErrorStack(r))
		}
	}()
	origin := m.timeline.idToTimelineItem.Keys()
	removed := make(map[int64]struct{})
	for _, i := range n {
		removed[int64(utils.InterfaceToInt(i))] = struct{}{}
	}
	allkeys := make([]int64, 0, len(origin))
	for _, originItem := range origin {
		if _, ok := removed[originItem]; !ok {
			allkeys = append(allkeys, originItem)
		}
	}
	stl := m.timeline.CreateSubTimeline(allkeys...)
	if stl == nil {
		return "no-toolcall, so not timeline"
	}
	return stl.Dump()
}

func (m *Memory) CurrentTaskTimeline() string {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error creating sub timeline: %v", r)
			fmt.Println(utils.ErrorStack(r))
		}
	}()
	if m.CurrentTask == nil {
		return m.Timeline()
	}
	stl := m.timeline.CreateSubTimeline(m.CurrentTask.toolCallResultIds.Keys()...)
	if stl == nil {
		return "no-toolcall, so not timeline"
	}
	return stl.Dump()
}

// timeline limit set
func (m *Memory) SetTimelineLimit(i int) {
	m.timeline.setTimelineLimit(i)
}

func (m *Memory) SetTimelineAICaller(caller AICaller) {

}

func (m *Memory) PromptForToolCallResultsForLastN(n int) string {
	return m.timeline.PromptForToolCallResultsForLastN(n)
}

//
//func (m *Memory) PromptForToolCallResultsForLast5() string {
//	return m.PromptForToolCallResultsForLastN(5)
//}
//
//func (m *Memory) PromptForToolCallResultsForLast10() string {
//	return m.PromptForToolCallResultsForLastN(10)
//}
//
//func (m *Memory) PromptForToolCallResultsForLast20() string {
//	return m.PromptForToolCallResultsForLastN(20)
//}

// memory tools current task info
func (m *Memory) CurrentTaskInfo() string {
	if m.CurrentTask == nil {
		return ""
	}
	templateData := map[string]interface{}{
		"Memory": m,
	}
	temp, err := template.New("current_task_info").Parse(__prompt_currentTaskInfo)
	if err != nil {
		log.Errorf("error parsing tool result history template: %v", err)
		return ""
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templateData)
	if err != nil {
		log.Errorf("error executing tool result history template: %v", err)
		return ""
	}
	return promptBuilder.String()
}

func (m *Memory) PersistentMemory() string {
	var buf bytes.Buffer
	buf.WriteString("# Now " + time.Now().String() + "\n")
	buf.WriteString("<persistent_memory>\n")
	m.PersistentData.ForEach(func(i string, v *PersistentDataRecord) bool {
		if v.Variable {
			buf.WriteString(fmt.Sprintf("%s: %s\n", i, v.Value))
		} else {
			buf.WriteString(fmt.Sprintf("%s\n", v.Value))
		}
		return true
	})
	buf.WriteString("</persistent_memory>\n")
	return buf.String()
}

func (m *Memory) PlanHelp() string {
	templateData := map[string]interface{}{
		"Memory": m,
	}
	temp, err := template.New("plan_help").Parse(__prompt_PlanHelp)
	if err != nil {
		log.Errorf("error parsing plan help template: %v", err)
		return ""
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templateData)
	if err != nil {
		log.Errorf("error executing plan help history template: %v", err)
		return ""
	}
	return promptBuilder.String()
}

func (m *Memory) ToolsList() string {
	if m.DisableTools {
		return ""
	}
	templateData := map[string]interface{}{
		"Tools": m.Tools(),
	}
	temp, err := template.New("tools_list").Parse(__prompt_ToolsList)
	if err != nil {
		log.Errorf("error parsing tool result history template: %v", err)
		return ""
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templateData)
	if err != nil {
		log.Errorf("error executing tool result history template: %v", err)
		return ""
	}
	return promptBuilder.String()
}

func (m *Memory) CurrentTaskToolCallResults() []*aitool.ToolResult {
	return m.CurrentTask.toolCallResultIds.Values()
}

func (m *Memory) StoreCliParameter(param []*ypb.ExecParamItem) {
	if utils.IsNil(m) {
		log.Warnf("memory nil while calling StoreCliParameter")
		return
	}
	for _, p := range param {
		if p.Key == "" {
			continue
		}
		m.SetPersistentData(p.Key, p.Value)
	}
}

func (m *Memory) SoftDeleteTimeline(id ...int64) {
	m.timeline.SoftDelete(id...)
}

func (m *Memory) ModifyMemoryFromOpList(opList ...aitool.InvokeParams) {
	for _, op := range opList {
		optype := op.GetString("op")
		opKey := op.GetString("key")
		opValue := op.GetString("value")
		switch optype {
		case "set":
			m.SetPersistentData(opKey, opValue)
		case "delete":
			m.DeletePersistentData(opKey)
		}
	}
}

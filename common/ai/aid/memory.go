package aid

import (
	"bytes"
	"fmt"
	osRuntime "runtime"
	"strings"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type PlanRecord struct { // todo
	PlanRequest  *planRequest
	PlanResponse *PlanResponse
}

type InteractiveEventRecord struct {
	InteractiveEvent *schema.AiOutputEvent
	UserInput        aitool.InvokeParams
}

type PersistentDataRecord struct {
	Variable bool
	Value    string
}

type PromptContextProvider struct {
	// user first input
	Query string

	// persistent data
	PersistentData *omap.OrderedMap[string, *PersistentDataRecord]

	// task info
	CurrentTask *AiTask
	RootTask    *AiTask

	// todo
	PlanHistory []*PlanRecord

	// tools list
	DisableTools          bool
	Tools                 func() []*aitool.Tool
	toolsKeywordsCallback func() []string

	// interactive history
	InteractiveHistory *omap.OrderedMap[string, *InteractiveEventRecord]

	timeline *aicommon.Timeline
}

func (m *PromptContextProvider) CopyReducibleMemory() *PromptContextProvider {
	mem := &PromptContextProvider{
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

	// Copy timeline if it exists
	if m.timeline != nil {
		mem.timeline = m.timeline.CopyReducibleTimelineWithMemory()
	} else {
		// Initialize with a new timeline if original is nil
		mem.timeline = aicommon.NewTimeline(nil, mem.CurrentTaskInfo)
	}

	return mem
}

func GetDefaultContextProvider() *PromptContextProvider {
	mem := &PromptContextProvider{
		PlanHistory:        make([]*PlanRecord, 0),
		PersistentData:     omap.NewOrderedMap[string, *PersistentDataRecord](make(map[string]*PersistentDataRecord)),
		InteractiveHistory: omap.NewOrderedMap[string, *InteractiveEventRecord](make(map[string]*InteractiveEventRecord)),
		Tools: func() []*aitool.Tool {
			return make([]*aitool.Tool, 0)
		},
	}
	mem.timeline = aicommon.NewTimeline(nil, mem.CurrentTaskInfo)
	return mem
}

func (m *PromptContextProvider) BindCoordinator(c *Coordinator) {
	config := c.Config
	m.StoreQuery(c.userInput)
	m.StoreTools(func() []*aitool.Tool {
		alltools, err := config.AiToolManager.GetEnableTools()
		if err != nil {
			log.Errorf("coordinator: get all tools failed: %v", err)
			return nil
		}
		return alltools
	})
	m.StoreToolsKeywords(func() []string {
		return config.Keywords
	})
	m.PushPersistentData(config.PersistentMemory...)
	m.timeline.BindConfig(config, config)
}

// user data memory api, user or ai can set and get

func (m *PromptContextProvider) PushPersistentData(values ...string) {
	for _, value := range values {
		m.PersistentData.Set(codec.Sha1(value), &PersistentDataRecord{
			Variable: false,
			Value:    value,
		})
	}
}

func (m *PromptContextProvider) SetPersistentData(key string, value string) {
	m.PersistentData.Set(key, &PersistentDataRecord{
		Variable: true,
		Value:    value,
	})
}

func (m *PromptContextProvider) GetPersistentData(key string) (string, bool) {
	res, ok := m.PersistentData.Get(key)
	if ok {
		return res.Value, ok
	}
	return "", ok
}

func (m *PromptContextProvider) DeletePersistentData(key string) {
	m.PersistentData.Delete(key)
}

// constants info memory api
func (m *PromptContextProvider) Now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func (m *PromptContextProvider) OS() string {
	return osRuntime.GOOS
}

func (m *PromptContextProvider) Arch() string {
	return osRuntime.GOARCH
}

func (m *PromptContextProvider) Schema() map[string]string {
	var toolNames []string
	for _, tool := range m.Tools() {
		toolNames = append(toolNames, tool.Name)
	}
	return planJSONSchema(toolNames)
}

// set tools list
func (m *PromptContextProvider) StoreTools(toolList func() []*aitool.Tool) {
	m.Tools = toolList
}

func (m *PromptContextProvider) ClearRuntimeConfig() {
	if m.timeline != nil {
		m.timeline.ClearRuntimeConfig()
	}
}

// TimelineDump returns the timeline dump safely
func (m *PromptContextProvider) TimelineDump() string {
	if m.timeline != nil {
		return m.timeline.Dump()
	}
	return ""
}

// set tools list
func (m *PromptContextProvider) StoreToolsKeywords(keywords func() []string) {
	m.toolsKeywordsCallback = keywords
}

func (m *PromptContextProvider) ToolsKeywords() string {
	return strings.Join(m.toolsKeywordsCallback(), ", ")
}

// user first input
func (m *PromptContextProvider) StoreQuery(query string) {
	m.Query = query
}

// task info memory
func (m *PromptContextProvider) StoreRootTask(t *AiTask) {
	m.RootTask = t
}

func (m *PromptContextProvider) Progress() string {
	if utils.IsNil(m) {
		return "empty *PromptContextProvider maybe a BUG"
	}
	if utils.IsNil(m.RootTask) {
		m.RootTask = m.CurrentTask.rootTask
	}
	return m.RootTask.Progress()
}

func (m *PromptContextProvider) StoreCurrentTask(task *AiTask) {
	m.CurrentTask = task
}

// interactive history memory
func (m *PromptContextProvider) StoreInteractiveEvent(eventID string, e *schema.AiOutputEvent) {
	// Check if there's already a placeholder record with user input
	if existing, ok := m.InteractiveHistory.Get(eventID); ok && existing.InteractiveEvent == nil {
		// Update the existing placeholder with the event
		existing.InteractiveEvent = e
		log.Debugf("updated placeholder interactive event record for ID %s", eventID)
	} else {
		// Create new record
		m.InteractiveHistory.Set(eventID, &InteractiveEventRecord{
			InteractiveEvent: e,
		})
	}
}

func (m *PromptContextProvider) StoreInteractiveUserInput(eventID string, invoke aitool.InvokeParams) {
	record, ok := m.InteractiveHistory.Get(eventID)
	if !ok {
		log.Debugf("interactive event record not found for ID %s, creating placeholder", eventID)
		// Create a placeholder record if it doesn't exist
		// This is normal in some timing scenarios
		m.InteractiveHistory.Set(eventID, &InteractiveEventRecord{
			InteractiveEvent: nil, // Will be set later if the event arrives
			UserInput:        invoke,
		})
		return
	}
	record.UserInput = invoke
}

// SafeStoreInteractiveUserInput safely stores interactive user input with better error handling
func (m *PromptContextProvider) SafeStoreInteractiveUserInput(eventID string, invoke aitool.InvokeParams) {
	if eventID == "" {
		log.Warn("attempted to store interactive user input with empty event ID")
		return
	}

	record, ok := m.InteractiveHistory.Get(eventID)
	if !ok {
		// Create a new record if it doesn't exist
		log.Infof("creating new interactive event record for ID %s", eventID)
		m.InteractiveHistory.Set(eventID, &InteractiveEventRecord{
			InteractiveEvent: nil, // Will be set when the event arrives
			UserInput:        invoke,
		})
		return
	}
	record.UserInput = invoke
	log.Debugf("updated interactive user input for event ID %s", eventID)
}

func (m *PromptContextProvider) GetInteractiveEventLast() (string, *InteractiveEventRecord, bool) {
	return m.InteractiveHistory.Last()
}

func (m *PromptContextProvider) GetInteractiveEvent(eventID string) (*InteractiveEventRecord, bool) {
	return m.InteractiveHistory.Get(eventID)
}

// tool results memory
func (m *PromptContextProvider) PushToolCallResults(t *aitool.ToolResult) {
	m.timeline.PushToolResult(t)
}

func (m *PromptContextProvider) PushUserInteraction(stage aicommon.UserInteractionStage, seq int64, question, userInput string) {
	m.timeline.PushUserInteraction(stage, seq, question, userInput)
}

func (m *PromptContextProvider) Timeline() string {
	return m.timeline.Dump()
}

func (m *PromptContextProvider) GetTimelineInstance() *aicommon.Timeline {
	return m.timeline
}

func (m *PromptContextProvider) SetTimelineInstance(timeline *aicommon.Timeline) {
	m.timeline = timeline
}

func (m *PromptContextProvider) PushText(id int64, i any) {
	m.timeline.PushText(id, utils.InterfaceToString(i))
}

func (m *PromptContextProvider) TimelineWithout(n ...any) string {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error creating sub timeline: %v", r)
			fmt.Println(utils.ErrorStack(r))
		}
	}()
	origin := m.timeline.GetTimelineItemIDs()
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

func (m *PromptContextProvider) CurrentTaskTimeline() string {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error creating sub timeline: %v", r)
			fmt.Println(utils.ErrorStack(r))
		}
	}()
	if m.CurrentTask == nil {
		return m.Timeline()
	}
	stl := m.timeline.CreateSubTimeline(m.CurrentTask.ToolCallResultsID()...)
	if stl == nil {
		return "no-toolcall, so not timeline"
	}
	return stl.Dump()
}

func (m *PromptContextProvider) TaskMaxContinue() int64 {
	return m.CurrentTask.Coordinator.MaxTaskContinue
}

// timeline limit set
func (m *PromptContextProvider) SetTimelineLimit(i int) {
	m.timeline.SetTimelineContentLimit(int64(i))
}

func (m *PromptContextProvider) PromptForToolCallResultsForLastN(n int) string {
	return m.timeline.PromptForToolCallResultsForLastN(n)
}

// memory tools current task info
func (m *PromptContextProvider) CurrentTaskInfo() string {
	if m.CurrentTask == nil {
		return "BUG:... currentTaskInfo cannot be generated in `CurrentTaskInfo`, no current task"
	}
	results, err := utils.RenderTemplate(__prompt_currentTaskInfo, map[string]interface{}{
		"ContextProvider": m,
	})
	if err != nil {
		return "BUG:... currentTaskInfo cannot be generated in `CurrentTaskInfo` err: " + err.Error()
	}

	return results
}

func (m *PromptContextProvider) PersistentMemory() string {
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

func (m *PromptContextProvider) PlanHelp() string {
	templateData := map[string]interface{}{
		"ContextProvider": m,
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

func (m *PromptContextProvider) ToolsList() string {
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

func (m *PromptContextProvider) CurrentTaskToolCallResults() []*aitool.ToolResult {
	return m.CurrentTask.GetAllToolCallResults()
}

func (m *PromptContextProvider) StoreCliParameter(param []*ypb.ExecParamItem) {
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

func (m *PromptContextProvider) SoftDeleteTimeline(id ...int64) {
	m.timeline.SoftDelete(id...)
}

var MemoryOpAction = "operate_memory"

var MemoryOpSchemaOption = []aitool.ToolOption{
	aitool.WithStringParam("@action", aitool.WithParam_Const(MemoryOpAction)), aitool.WithStructArrayParam("memory_op",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("persistent memory operation, you can use this to store some data in the memory, and it will be used in the next call"),
			aitool.WithParam_Required(true),
			aitool.WithParam_MinLength(0),
		},
		nil,
		aitool.WithStringParam("op", aitool.WithParam_Description("the operation type, can be 'set', 'delete'"), aitool.WithParam_EnumString("set", "delete"), aitool.WithParam_Required(true)),
		aitool.WithStringParam("key", aitool.WithParam_Description("the key of the persistent memory, if you set op to 'set', this is required"), aitool.WithParam_Required(true)),
		aitool.WithStringParam("value", aitool.WithParam_Description("the value of the persistent memory, if you set op to 'set', this is required"), aitool.WithParam_Required(false)),
	),
}

// ApplyOp applies a list of operations to the memory.
func (m *PromptContextProvider) ApplyOp(memoryOpAction *aicommon.Action) {
	if memoryOpAction.ActionType() != MemoryOpAction {
		return
	}
	opList := memoryOpAction.GetInvokeParamsArray("memory_op")
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

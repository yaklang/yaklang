package aibp

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/go-rod/rod/lib/utils"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"gotest.tools/v3/assert"
)

var planJson = `{
  "@action": "plan",
  "main_task": "计算1+1的值",
  "main_task_goal": "计算1+1的值",
  "tasks": [
    {
      "subtask_name": "计算1+1的值",
      "subtask_goal": "计算1+1的值"
    }
  ]
}`
var finishJson = `{
  "@action": "direct-answer",
  "status_summary": "已完成计算1+1的值任务，成功计算了1+1的值。",
  "task_long_summary": "本次任务完成了对1+1的值的计算。通过计算，我们得到了1+1的值为2。",
  "task_short_summary": "完成计算1+1的值任务，成功计算了1+1的值。",
  "direct_answer": "result is 2",
  "direct_answer_long": "本次任务完成了对1+1的值的计算。通过计算，我们得到了1+1的值为2。",
  "shrink_similar_tool_call_result": "",
  "summary_tool_call_result": ""
}`

var summaryJson = `result is 2`

func MockAICallback(t *testing.T, initFlag, persistentFlag, planFlag string) aicommon.AICallbackType {
	step := 0
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()
		defer rsp.Close()
		switch step {
		case 0:
			if initFlag != "" && !strings.Contains(req.GetPrompt(), initFlag) {
				t.Fatalf("init flag not found in prompt: %s", req.GetPrompt())
			}
			if planFlag != "" && !strings.Contains(req.GetPrompt(), planFlag) {
				t.Fatalf("plan flag not found in prompt: %s", req.GetPrompt())
			}
			rsp.EmitOutputStream(strings.NewReader(planJson))
		case 1:
			if persistentFlag != "" && !strings.Contains(req.GetPrompt(), persistentFlag) {
				t.Fatalf("persistent flag not found in prompt: %s", req.GetPrompt())
			}
			rsp.EmitOutputStream(strings.NewReader(finishJson))
		default:
		}
		step++
		return rsp, nil
	}
}

func RunTestForge(t *testing.T, forge *schema.AIForge, initFlag, persistentFlag string) (any, error) {
	db := consts.GetGormProfileDatabase()
	forge.IsTemporary = true
	err := yakit.CreateOrUpdateAIForgeByName(db, forge.ForgeName, forge)
	if err != nil {
		return nil, err
	}

	defer func() {
		yakit.DeleteAIForgeByName(db, forge.ForgeName)
	}()

	result, err := yak.ExecuteForge(forge.ForgeName, map[string]any{
		"query": "1+1",
	},
		aicommon.WithAICallback(MockAICallback(t, initFlag, persistentFlag, "")),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDebugPrompt(true),
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func TestBuildForgeFromYak(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName: forgeName,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
init = "帮我计算表达式的值` + initFlag + `"
persis = "一定要算准一点` + persistentFlag + `"

forgeHandle = func(params) {
	result = ""
    bp = aiagent.CreateForge("add",
        aiagent.persistentPrompt(persis),
		aiagent.initPrompt(init),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			result = config.GetMemory().CurrentTask.TaskSummary
		}),
    )
    ordr,err = bp.CreateCoordinator(context.Background(),params)
    if err != nil {
		return nil
	}
    err = ordr.Run()
    if err != nil {
		return nil
	}
		println(result)
    return result
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}

func TestBuildForgeFromYakWithDefaultPrompt(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()

forgeHandle = func(params) {
	result = ""
    bp = aiagent.CreateForge("add",
        aiagent.persistentPrompt(__PERSISTENT_PROMPT__),
		aiagent.initPrompt(__INIT_PROMPT__),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			result = config.GetMemory().CurrentTask.TaskSummary
		}),
    )
    ordr,err = bp.CreateCoordinator(context.Background(),params)
    if err != nil {
		return nil
	}
    err = ordr.Run()
    if err != nil {
		return nil
	}
    return result
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}

func TestBuildForgeFromYakWithDefaultForgeHandle(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestBuildForgeFromYakWithWarpDefaultForgeHandle(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
forgeHandle = func(params,opts...) {
	res,_ = __DEFAULT_FORGE_HANDLE__(params,opts...)
	res.Formated = "result is 2"
	return res
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	forgeResult, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
	assert.Equal(t, forgeResult.Formated, "result is 2")
}
func TestBuildForgeFromYakWithRewriteDefaultPrompt(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName: forgeName,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
__INIT_PROMPT__ = "帮我计算表达式的值" + "` + initFlag + `"
__PERSISTENT_PROMPT__ = "一定要算准一点" + "` + persistentFlag + `"
`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestBuildForgeFromYakNoCheck(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent:     `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestBuildForgeFromYakNoQuery(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestNewForgeExecutor(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
forgeHandle = func(params,opts...) {
	res = ""
	opts = append(opts,
		aiagent.persistentPrompt(__PERSISTENT_PROMPT__),
		aiagent.initPrompt(__INIT_PROMPT__),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			res = config.GetMemory().CurrentTask.TaskSummary
		}),
	)
	excutor,err := aiagent.NewExecutor("test",params,opts...)
	if err != nil {
		return nil
	}
	err = excutor.Run()
	if err != nil {
		return nil
	}
	return res
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}

func TestNewForgeExecutorFromJson(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandString(10)
	forgeData := map[string]any{
		"name":              forgeName,
		"persistent_prompt": "一定要算准一点" + persistentFlag,
		"init_prompt":       "帮我计算表达式的值" + initFlag,
		"forge_content":     "query = cli.String(\"query\", cli.setRequired(true), cli.setHelp(\"query\"))",
	}
	jsonData, err := json.Marshal(forgeData)
	if err != nil {
		t.Errorf("marshal forge data failed: %v", err)
		return
	}
	jsonForge := strconv.Quote(string(jsonData))
	forge := &schema.AIForge{
		ForgeName: forgeName,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
forgeHandle = func(params,opts...) {
	res = ""
	opts = append(opts,
		aiagent.persistentPrompt(__PERSISTENT_PROMPT__),
		aiagent.initPrompt(__INIT_PROMPT__),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			res = config.GetMemory().CurrentTask.TaskSummary
		}),
	)
	excutor,err := aiagent.NewExecutorFromJson(` + jsonForge + `,params,opts...)
	if err != nil {
		return nil
	}
	err = excutor.Run()
	if err != nil {
		return nil
	}
	return res
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}

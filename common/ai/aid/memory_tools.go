package aid

import (
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (m *PromptContextProvider) CreateBasicMemoryTools() ([]*aitool.Tool, error) {
	var err error
	factory := aitool.NewFactory()
	err = factory.RegisterTool("delete_memory",
		aitool.WithDescription("delete ai task memory from timeline by id"),
		aitool.WithDangerousNoNeedTimelineRecorded(true),
		aitool.WithIntegerParam(
			"id",
			aitool.WithParam_Required(true),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			id := params.GetInt("id")
			m.SoftDeleteTimeline(id)
			return nil, nil
		}))
	if err != nil {
		log.Errorf("register memory_query tool: %v", err)
	}
	err = factory.RegisterTool("add_persistent_memory",
		aitool.WithDescription("add persistent memory, which will be appended to every prompt"),
		aitool.WithDangerousNoNeedTimelineRecorded(true),
		aitool.WithStringParam("content",
			aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			content := params.GetString("content")
			m.PushPersistentData(content)
			return nil, nil
		}))
	if err != nil {
		log.Errorf("register memory_progress tool: %v", err)
	}
	return factory.Tools(), nil
}

// Memory 和 config的 绑定太强烈了，所以将tool直接放置在aid包内
func (m *PromptContextProvider) CreateMemoryTools() ([]*aitool.Tool, error) {
	var err error
	factory := aitool.NewFactory()

	err = factory.RegisterTool("memory_query",
		aitool.WithDescription("get ai task user first query content"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return m.Query, nil
		}))
	if err != nil {
		log.Errorf("register memory_query tool: %v", err)
	}
	err = factory.RegisterTool("memory_progress",
		aitool.WithDescription("get ai task progress"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return m.CurrentTask.Progress(), nil
		}))
	if err != nil {
		log.Errorf("register memory_progress tool: %v", err)
	}

	err = factory.RegisterTool("memory_tool_call_results",
		aitool.WithDescription("get ai task tool call results"),
		aitool.WithIntegerParam("lastN", aitool.WithParam_Description("last n tool call results"), aitool.WithParam_Default(20)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			lastN := params.GetInt("lastN")
			return m.PromptForToolCallResultsForLastN(int(lastN)), nil
		}))
	if err != nil {
		log.Errorf("register memory_tool_call_results tool: %v", err)
	}

	// user data api, user or ai can write and read
	err = factory.RegisterTool("memory_persistent_data_set",
		aitool.WithDescription("memory tools: set user data to memory;user data  as the AI's external long-term memory, allowing the AI to read from and write to this data storage to maintain continuity across different contexts. "),
		aitool.WithStringParam("key", aitool.WithParam_Required(true), aitool.WithParam_Description("user data key")),
		aitool.WithStringParam("value", aitool.WithParam_Required(true), aitool.WithParam_Description("user data value")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			key := params.GetString("key")
			value := params.GetString("value")
			m.SetPersistentData(key, value)
			return nil, nil
		}))
	if err != nil {
		log.Errorf("register memory_persistent_data_set tool: %v", err)
	}

	err = factory.RegisterTool("memory_persistent_data_get",
		aitool.WithDescription("memory tools: get user data in memory; user data  as the AI's external long-term memory, allowing the AI to read from and write to this data storage to maintain continuity across different contexts"),
		aitool.WithStringParam("key", aitool.WithParam_Required(true), aitool.WithParam_Description("user data key")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			key := params.GetString("key")
			value, ok := m.GetPersistentData(key)
			if !ok {
				return nil, utils.Error("get memory user data fail: key not found")
			}
			return value, nil
		}))
	if err != nil {
		log.Errorf("register memory_persistent_data_get tool: %v", err)
	}

	err = factory.RegisterTool("memory_persistent_data_delete",
		aitool.WithDescription("memory tools: delete user data from memory; user data  as the AI's external long-term memory, allowing the AI to read from and write to this data storage to maintain continuity across different contexts"),
		aitool.WithStringParam("key", aitool.WithParam_Required(true), aitool.WithParam_Description("user data key")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			key := params.GetString("key")
			m.DeletePersistentData(key)
			return nil, nil
		}))
	if err != nil {
		log.Errorf("register memory_persistent_data_delete tool: %v", err)
	}
	err = factory.RegisterTool("memory_persistent_data_list",
		aitool.WithDescription("memory tools: list user data key in memory; user data  as the AI's external long-term memory, allowing the AI to read from and write to this data storage to maintain continuity across different contexts"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return m.PersistentData.Keys(), nil
		}))
	if err != nil {
		log.Errorf("register memory_user_data_list tool: %v", err)
	}

	tools := factory.Tools()
	if len(tools) == 0 {
		return nil, utils.Errorf("no tools registered")
	}
	return tools, nil
}

package aid

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"testing"
)

func Test_MemoryTools(t *testing.T) {
	m := GetDefaultMemory()
	tools, err := m.CreateMemoryTools()
	if err != nil {
		return
	}

	var userDataSet, userDataGet, userDataDelete, userDataList *aitool.Tool
	var userQuery *aitool.Tool

	for _, tool := range tools {
		name := tool.Name
		switch name {
		case "memory_user_data_set":
			userDataSet = tool
		case "memory_user_data_get":
			userDataGet = tool
		case "memory_user_data_delete":
			userDataDelete = tool
		case "memory_user_data_list":
			userDataList = tool
		case "memory_query":
			userQuery = tool
		default:
		}
	}

	// test user data
	tokenKey := uuid.New().String()
	tokenValue := uuid.New().String()
	_, err = userDataSet.InvokeWithParams(map[string]any{
		"key":   tokenKey,
		"value": tokenValue,
	})
	require.NoError(t, err)

	callRes, err := userDataList.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, callRes.Data.(*aitool.ToolExecutionResult).Result, 1)
	require.Contains(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenKey)

	callRes, err = userDataGet.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.NoError(t, err)
	require.Equal(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenValue)

	_, err = userDataDelete.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.NoError(t, err)

	callRes, err = userDataList.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, callRes.Data.(*aitool.ToolExecutionResult).Result, 0)

	_, err = userDataGet.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.Error(t, err)

	tokenQuery := uuid.New().String()
	m.StoreQuery(tokenQuery)
	callRes, err = userQuery.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Equal(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenQuery)

}

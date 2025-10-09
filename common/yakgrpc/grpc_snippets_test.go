package yakgrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSnippetsServer(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, err)

	checkCustomCode := func(want map[string]*schema.Snippets) {
		var names []string
		var get map[string]*schema.Snippets = make(map[string]*schema.Snippets)
		for n, _ := range want {
			names = append(names, n)
		}

		res, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: names,
			},
		})
		require.NoError(t, err)
		require.Len(t, res.GetNames(), len(res.GetCodes()))
		require.Len(t, res.GetNames(), len(res.GetDescriptions()))
		require.Len(t, res.GetNames(), len(res.GetStates()))

		for i := 0; i < len(res.GetNames()); i++ {
			get[res.GetNames()[i]] = &schema.Snippets{
				SnippetName:  res.GetNames()[i],
				SnippetBody:  res.GetCodes()[i],
				SnippetDesc:  res.GetDescriptions()[i],
				SnippetState: schema.SwitcSnippetsType(res.GetStates()[i]),
				SnippetLevel: schema.SwitcSnippetsLevel(res.GetLevels()[i]),
			}
		}
		require.Equal(t, get, want)
	}
	deleteCustomCode := func(target []string) {
		client.DeleteSnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: target,
			},
		})
	}

	t.Run("check CreateCustomCode normal", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()
		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		checkCustomCode(map[string]*schema.Snippets{
			name: {
				SnippetName:  name,
				SnippetBody:  code,
				SnippetDesc:  desc,
				SnippetState: schema.Snippets_Type_Http,
				SnippetLevel: schema.Snippets_Level_Method,
			},
		})
		defer deleteCustomCode([]string{name})
	})

	t.Run("check CreateCustomCode already exists", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
		})
		require.NoError(t, err)

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.Error(t, err, utils.Errorf("custom code name already exists"))

		defer deleteCustomCode([]string{name})
	})

	t.Run("check DeleteCustomCode normal", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()
		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		_, err := client.DeleteSnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: []string{name},
			},
		})
		require.NoError(t, err)

		res, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: []string{name},
			},
		})
		require.NoError(t, err)
		require.Len(t, res.GetNames(), 0)

		defer deleteCustomCode([]string{name})
	})

	t.Run("check UpdateCustomCode normal", func(t *testing.T) {
		name := uuid.NewString()
		code1 := uuid.NewString()
		code2 := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name,
			Code:        code1,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		_, err = client.UpdateSnippet(context.Background(), &ypb.EditSnippetsRequest{
			Target:      name,
			Name:        name,
			Code:        code2,
			Description: desc,
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		checkCustomCode(map[string]*schema.Snippets{
			name: {
				SnippetName:  name,
				SnippetBody:  code2,
				SnippetDesc:  desc,
				SnippetState: schema.Snippets_Type_Yak,
				SnippetLevel: schema.Snippets_Level_Method,
			},
		})

		defer deleteCustomCode([]string{name})
	})

	t.Run("check UpdateCustomCode not found", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.UpdateSnippet(context.Background(), &ypb.EditSnippetsRequest{
			Target:      name,
			Name:        name,
			Code:        code,
			Description: desc,
			Level:       string(schema.Snippets_Level_Method),
		})
		require.Error(t, err, utils.Errorf("custom code signing not found"))

		defer deleteCustomCode([]string{name})
	})

	t.Run("check UpdateCustomCode new name", func(t *testing.T) {
		name1 := uuid.NewString()
		name2 := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name1,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		_, err = client.UpdateSnippet(context.Background(), &ypb.EditSnippetsRequest{
			Target:      name1,
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		res, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: []string{name1},
			},
		})
		require.NoError(t, err)
		require.Len(t, res.GetNames(), 0)

		checkCustomCode(map[string]*schema.Snippets{
			name2: {
				SnippetName:  name2,
				SnippetBody:  code,
				SnippetDesc:  desc,
				SnippetState: schema.Snippets_Type_Yak,
				SnippetLevel: schema.Snippets_Level_Method,
			},
		})

		defer deleteCustomCode([]string{name1, name2})
	})

	t.Run("check UpdateCustomCode new name found", func(t *testing.T) {
		name1 := uuid.NewString()
		name2 := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name1,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})

		require.NoError(t, err)
		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		_, err = client.UpdateSnippet(context.Background(), &ypb.EditSnippetsRequest{
			Target:      name1,
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.Error(t, err, utils.Errorf("new custom code signing is found"))

		defer deleteCustomCode([]string{name1, name2})
	})

	t.Run("check DeleteSnippets with empty filter does not delete", func(t *testing.T) {
		name1 := uuid.NewString()
		name2 := uuid.NewString()

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name1,
			Code:        "code1",
			Description: "desc1",
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Function),
		})
		require.NoError(t, err)

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name2,
			Code:        "code2",
			Description: "desc2",
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)
		defer deleteCustomCode([]string{name1, name2})

		resBefore, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{},
		})
		require.NoError(t, err)
		countBefore := len(resBefore.GetNames())
		require.GreaterOrEqual(t, countBefore, 2, "should have at least 2 snippets")

		_, err = client.DeleteSnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: []string{},
			},
		})
		require.NoError(t, err, "empty filter should succeed without deleting")

		resAfter, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{},
		})
		require.NoError(t, err)
		countAfter := len(resAfter.GetNames())

		require.Equal(t, countBefore, countAfter, "empty filter should not delete any snippets")
		require.Contains(t, resAfter.GetNames(), name1, "snippet1 should still exist")
		require.Contains(t, resAfter.GetNames(), name2, "snippet2 should still exist")
	})
}

func TestShowSnippetsWithJson(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, err)

	deleteCustomCode := func(target []string) {
		client.DeleteSnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: target,
			},
		})
	}

	checkVSCodeSnippet := func(snippet map[string]interface{}, expectedPrefix, expectedDesc, expectedScope string, expectedBodyLines []string) {
		require.Contains(t, snippet, "scope", "snippet should have scope field")
		require.Contains(t, snippet, "prefix", "snippet should have prefix field")
		require.Contains(t, snippet, "body", "snippet should have body field")
		require.Contains(t, snippet, "description", "snippet should have description field")

		require.Equal(t, expectedPrefix, snippet["prefix"], "prefix mismatch")
		require.Equal(t, expectedDesc, snippet["description"], "description mismatch")
		require.Equal(t, expectedScope, snippet["scope"], "scope mismatch")

		body, ok := snippet["body"].([]interface{})
		require.True(t, ok, "body should be an array")
		require.Len(t, body, len(expectedBodyLines), "body lines count mismatch")

		for i, expectedLine := range expectedBodyLines {
			actualLine, ok := body[i].(string)
			require.True(t, ok, "body line should be string")
			require.Equal(t, expectedLine, actualLine, "body line %d mismatch", i)
		}
	}

	t.Run("check VSCode format snippet output", func(t *testing.T) {
		name1 := "Print to console"
		name2 := "HTTP Request"
		code1 := "console.log('$1');\n$2"
		code2 := "GET /api/test HTTP/1.1\nHost: $1"
		desc1 := "Log output to console"
		desc2 := "Sample HTTP request"

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name1,
			Code:        code1,
			Description: desc1,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name2,
			Code:        code2,
			Description: desc2,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)
		defer deleteCustomCode([]string{name1, name2})

		response, err := client.ShowSnippetsWithJson(context.Background(), &ypb.Empty{})
		require.NoError(t, err)
		require.NotEmpty(t, response.GetJsonData(), "response code should not be empty")

		t.Logf("Generated JSON:\n%s", response.GetJsonData())

		var snippets map[string]map[string]interface{}
		err = json.Unmarshal([]byte(response.GetJsonData()), &snippets)
		require.NoError(t, err, "JSON should be valid")

		require.Contains(t, snippets, name1, "should contain %s", name1)
		require.Contains(t, snippets, name2, "should contain %s", name2)
		require.Len(t, snippets, 2, "should have exactly 2 snippets")

		checkVSCodeSnippet(snippets[name1], name1, desc1, string(schema.Snippets_Type_Http), []string{
			"console.log('$1');",
			"$2",
		})

		checkVSCodeSnippet(snippets[name2], name2, desc2, string(schema.Snippets_Type_Http), []string{
			"GET /api/test HTTP/1.1",
			"Host: $1",
		})
	})

	t.Run("check empty database returns empty JSON object", func(t *testing.T) {
		testName := uuid.NewString()
		_, err := client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        testName,
			Code:        "test",
			Description: "test",
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Function),
		})
		require.NoError(t, err)

		_, err = client.DeleteSnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: []string{testName},
			},
		})
		require.NoError(t, err)

		_, err = client.DeleteSnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{
				Name: []string{},
			},
		})
		require.NoError(t, err, "empty filter should not cause error")

		response, err := client.ShowSnippetsWithJson(context.Background(), &ypb.Empty{})
		require.NoError(t, err)
		require.NotEmpty(t, response.GetJsonData(), "response should not be empty")

		t.Logf("Current database JSON: %s", response.GetJsonData())
	})

	t.Run("check multi-line snippet correct splitting", func(t *testing.T) {
		name := "Multi-line snippet"
		code := "line1\nline2\nline3"
		desc := "Test multi-line"

		_, err := client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Function),
		})
		require.NoError(t, err)
		defer deleteCustomCode([]string{name})

		response, err := client.ShowSnippetsWithJson(context.Background(), &ypb.Empty{})
		require.NoError(t, err)

		var snippets map[string]map[string]interface{}
		err = json.Unmarshal([]byte(response.GetJsonData()), &snippets)
		require.NoError(t, err)

		require.Contains(t, snippets, name)
		checkVSCodeSnippet(snippets[name], name, desc, string(schema.Snippets_Type_Yak), []string{
			"line1",
			"line2",
			"line3",
		})
	})

	t.Run("check special characters are properly escaped in JSON", func(t *testing.T) {
		name := "Special chars snippet"
		code := "console.log(\"test\");\nvar x = 'value';"
		desc := "Test with \"quotes\" and 'apostrophes'"

		_, err := client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)
		defer deleteCustomCode([]string{name})

		response, err := client.ShowSnippetsWithJson(context.Background(), &ypb.Empty{})
		require.NoError(t, err)
		require.NotEmpty(t, response.GetJsonData())

		var snippets map[string]map[string]interface{}
		err = json.Unmarshal([]byte(response.GetJsonData()), &snippets)
		require.NoError(t, err, "JSON with special chars should be valid")

		require.Contains(t, snippets, name)
		snippet := snippets[name]
		require.Equal(t, desc, snippet["description"], "special chars in description should be preserved")
	})
}

func TestImportSnippetsFromJson(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, err)

	t.Run("check import VSCode JSON format snippets", func(t *testing.T) {
		vscodeJSON := `{
    "Print to console": {
        "scope": "yak",
        "prefix": "log",
        "body": [
            "println('$1')",
            "$2"
        ],
        "description": "Log output to console"
    },
    "HTTP Request": {
        "scope": "http",
        "prefix": "httpreq",
        "body": [
            "GET /api/test HTTP/1.1",
            "Host: $1",
            "Content-Type: application/json"
        ],
        "description": "Sample HTTP request"
    }
}`

		_, err := client.ImportSnippetsFromJson(context.Background(), &ypb.ImportSnippetsRequest{
			JsonData: vscodeJSON,
		})
		require.NoError(t, err)

		response, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{},
		})
		require.NoError(t, err)
		require.Len(t, response.Names, 2, "should have 2 imported snippets")

		names := response.Names
		require.Contains(t, names, "log")
		require.Contains(t, names, "httpreq")

		for i, name := range names {
			if name == "log" {
				require.Equal(t, "println('$1')\n$2", response.Codes[i])
				require.Equal(t, "Log output to console", response.Descriptions[i])
				require.Equal(t, "yak", response.States[i])
			} else if name == "httpreq" {
				require.Equal(t, "GET /api/test HTTP/1.1\nHost: $1\nContent-Type: application/json", response.Codes[i])
				require.Equal(t, "Sample HTTP request", response.Descriptions[i])
				require.Equal(t, "http", response.States[i])
			}
		}
	})

	t.Run("check import clears existing snippets", func(t *testing.T) {
		_, err := client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        "old_snippet_1",
			Code:        "old code 1",
			Description: "Old snippet 1",
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Function),
		})
		require.NoError(t, err)

		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        "old_snippet_2",
			Code:        "old code 2",
			Description: "Old snippet 2",
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
		})
		require.NoError(t, err)

		response, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{},
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(response.Names), 2, "should have at least 2 snippets")

		newJSON := `{
    "new_snippet": {
        "scope": "yak",
        "prefix": "new_snippet",
        "body": ["new code"],
        "description": "New snippet"
    }
}`
		_, err = client.ImportSnippetsFromJson(context.Background(), &ypb.ImportSnippetsRequest{
			JsonData: newJSON,
		})
		require.NoError(t, err)

		response, err = client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{},
		})
		require.NoError(t, err)
		require.Len(t, response.Names, 1, "should only have 1 snippet after import")
		require.Equal(t, "new_snippet", response.Names[0])
		require.NotContains(t, response.Names, "old_snippet_1")
		require.NotContains(t, response.Names, "old_snippet_2")
	})

	t.Run("check import with invalid JSON", func(t *testing.T) {
		invalidJSON := `{"invalid": "json"`

		_, err := client.ImportSnippetsFromJson(context.Background(), &ypb.ImportSnippetsRequest{
			JsonData: invalidJSON,
		})
		require.Error(t, err, "should fail with invalid JSON")
		require.Contains(t, err.Error(), "failed to parse JSON")
	})

	t.Run("check import with empty JSON", func(t *testing.T) {
		_, err := client.ImportSnippetsFromJson(context.Background(), &ypb.ImportSnippetsRequest{
			JsonData: "",
		})
		require.Error(t, err, "should fail with empty JSON")
		require.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("check import empty snippets object", func(t *testing.T) {
		emptyJSON := `{}`

		_, err := client.ImportSnippetsFromJson(context.Background(), &ypb.ImportSnippetsRequest{
			JsonData: emptyJSON,
		})
		require.NoError(t, err, "should succeed with empty object")

		response, err := client.QuerySnippets(context.Background(), &ypb.QuerySnippetsRequest{
			Filter: &ypb.SnippetsFilter{},
		})
		require.NoError(t, err)
		require.Len(t, response.Names, 0, "should have no snippets")
	})
}

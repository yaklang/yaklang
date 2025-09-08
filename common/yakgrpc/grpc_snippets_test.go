package yakgrpc

import (
	"context"
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
				SnippetName:   res.GetNames()[i],
				SnippetBody:   res.GetCodes()[i],
				SnippetDesc:   res.GetDescriptions()[i],
				SnippetState:  schema.SwitcSnippetsType(res.GetStates()[i]),
				SnippetLevel:  schema.SwitcSnippetsLevel(res.GetLevels()[i]),
				SnippetPrefix: res.GetPrefixs()[i],
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
			Prefix:      name,
		})
		require.NoError(t, err)

		checkCustomCode(map[string]*schema.Snippets{
			name: {
				SnippetName:   name,
				SnippetBody:   code,
				SnippetDesc:   desc,
				SnippetState:  schema.Snippets_Type_Http,
				SnippetLevel:  schema.Snippets_Level_Method,
				SnippetPrefix: name,
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
			Prefix:      name,
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
			Prefix:      name,
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
			Prefix:      name,
		})
		require.NoError(t, err)

		_, err = client.UpdateSnippet(context.Background(), &ypb.EditSnippetsRequest{
			Target:      name,
			Name:        name,
			Code:        code2,
			Description: desc,
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Method),
			Prefix:      name,
		})
		require.NoError(t, err)

		checkCustomCode(map[string]*schema.Snippets{
			name: {
				SnippetName:   name,
				SnippetBody:   code2,
				SnippetDesc:   desc,
				SnippetState:  schema.Snippets_Type_Yak,
				SnippetLevel:  schema.Snippets_Level_Method,
				SnippetPrefix: name,
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
			Prefix:      name,
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
			Prefix:      name1,
		})
		require.NoError(t, err)

		_, err = client.UpdateSnippet(context.Background(), &ypb.EditSnippetsRequest{
			Target:      name1,
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Method),
			Prefix:      name2,
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
				SnippetName:   name2,
				SnippetBody:   code,
				SnippetDesc:   desc,
				SnippetState:  schema.Snippets_Type_Yak,
				SnippetLevel:  schema.Snippets_Level_Method,
				SnippetPrefix: name2,
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
			Prefix:      name1,
		})

		require.NoError(t, err)
		_, err = client.CreateSnippet(context.Background(), &ypb.SnippetsRequest{
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Http),
			Level:       string(schema.Snippets_Level_Method),
			Prefix:      name2,
		})
		require.NoError(t, err)

		_, err = client.UpdateSnippet(context.Background(), &ypb.EditSnippetsRequest{
			Target:      name1,
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.Snippets_Type_Yak),
			Level:       string(schema.Snippets_Level_Method),
			Prefix:      name2,
		})
		require.Error(t, err, utils.Errorf("new custom code signing is found"))

		defer deleteCustomCode([]string{name1, name2})
	})
}

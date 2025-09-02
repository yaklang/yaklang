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

func TestCustomCodeServer(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, err)

	checkCustomCode := func(want map[string]*schema.CustomCodeSigning) {
		var names []string
		var get map[string]*schema.CustomCodeSigning = make(map[string]*schema.CustomCodeSigning)
		for n, _ := range want {
			names = append(names, n)
		}

		res, err := client.QueryCustomCode(context.Background(), &ypb.QueryCustomCodeRequest{
			Filter: &ypb.CustomCodeFilter{
				Name: names,
			},
		})
		require.NoError(t, err)
		require.Len(t, res.GetNames(), len(res.GetCodes()))
		require.Len(t, res.GetNames(), len(res.GetDescriptions()))
		require.Len(t, res.GetNames(), len(res.GetStates()))

		for i := 0; i < len(res.GetNames()); i++ {
			get[res.GetNames()[i]] = &schema.CustomCodeSigning{
				CustomCodeName:  res.GetNames()[i],
				CustomCodeData:  res.GetCodes()[i],
				CustomCodeDesc:  res.GetDescriptions()[i],
				CustomCodeState: schema.SwitcCustomCodeSigningType(res.GetStates()[i]),
				CustomCodeLevel: schema.SwitcCustomCodeSigningLevel(res.GetLevels()[i]),
			}
		}
		require.Equal(t, get, want)
	}
	deleteCustomCode := func(target []string) {
		client.DeleteCustomCode(context.Background(), &ypb.QueryCustomCodeRequest{
			Filter: &ypb.CustomCodeFilter{
				Name: target,
			},
		})
	}

	t.Run("check CreateCustomCode normal", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()
		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.NoError(t, err)

		checkCustomCode(map[string]*schema.CustomCodeSigning{
			name: {
				CustomCodeName:  name,
				CustomCodeData:  code,
				CustomCodeDesc:  desc,
				CustomCodeState: schema.CustomCodeSigning_Type_Http,
				CustomCodeLevel: schema.CustomCodeSigning_Level_Method,
			},
		})
		defer deleteCustomCode([]string{name})
	})

	t.Run("check CreateCustomCode already exists", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
		})
		require.NoError(t, err)

		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.Error(t, err, utils.Errorf("custom code name already exists"))

		defer deleteCustomCode([]string{name})
	})

	t.Run("check DeleteCustomCode normal", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()
		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.NoError(t, err)

		_, err := client.DeleteCustomCode(context.Background(), &ypb.QueryCustomCodeRequest{
			Filter: &ypb.CustomCodeFilter{
				Name: []string{name},
			},
		})
		require.NoError(t, err)

		res, err := client.QueryCustomCode(context.Background(), &ypb.QueryCustomCodeRequest{
			Filter: &ypb.CustomCodeFilter{
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

		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name,
			Code:        code1,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.NoError(t, err)

		_, err = client.UpdateCustomCode(context.Background(), &ypb.EditCustomCodeRequest{
			Target:      name,
			Name:        name,
			Code:        code2,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Yak),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.NoError(t, err)

		checkCustomCode(map[string]*schema.CustomCodeSigning{
			name: {
				CustomCodeName:  name,
				CustomCodeData:  code2,
				CustomCodeDesc:  desc,
				CustomCodeState: schema.CustomCodeSigning_Type_Yak,
				CustomCodeLevel: schema.CustomCodeSigning_Level_Method,
			},
		})

		defer deleteCustomCode([]string{name})
	})

	t.Run("check UpdateCustomCode not found", func(t *testing.T) {
		name := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.UpdateCustomCode(context.Background(), &ypb.EditCustomCodeRequest{
			Target:      name,
			Name:        name,
			Code:        code,
			Description: desc,
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.Error(t, err, utils.Errorf("custom code signing not found"))

		defer deleteCustomCode([]string{name})
	})

	t.Run("check UpdateCustomCode new name", func(t *testing.T) {
		name1 := uuid.NewString()
		name2 := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name1,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.NoError(t, err)

		_, err = client.UpdateCustomCode(context.Background(), &ypb.EditCustomCodeRequest{
			Target:      name1,
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Yak),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.NoError(t, err)

		res, err := client.QueryCustomCode(context.Background(), &ypb.QueryCustomCodeRequest{
			Filter: &ypb.CustomCodeFilter{
				Name: []string{name1},
			},
		})
		require.NoError(t, err)
		require.Len(t, res.GetNames(), 0)

		checkCustomCode(map[string]*schema.CustomCodeSigning{
			name2: {
				CustomCodeName:  name2,
				CustomCodeData:  code,
				CustomCodeDesc:  desc,
				CustomCodeState: schema.CustomCodeSigning_Type_Yak,
				CustomCodeLevel: schema.CustomCodeSigning_Level_Method,
			},
		})

		defer deleteCustomCode([]string{name1, name2})
	})

	t.Run("check UpdateCustomCode new name found", func(t *testing.T) {
		name1 := uuid.NewString()
		name2 := uuid.NewString()
		code := uuid.NewString()
		desc := uuid.NewString()

		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name1,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})

		require.NoError(t, err)
		_, err = client.CreateCustomCode(context.Background(), &ypb.CustomCodeRequest{
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Http),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.NoError(t, err)

		_, err = client.UpdateCustomCode(context.Background(), &ypb.EditCustomCodeRequest{
			Target:      name1,
			Name:        name2,
			Code:        code,
			Description: desc,
			State:       string(schema.CustomCodeSigning_Type_Yak),
			Level:       string(schema.CustomCodeSigning_Level_Method),
		})
		require.Error(t, err, utils.Errorf("new custom code signing is found"))

		defer deleteCustomCode([]string{name1, name2})
	})
}

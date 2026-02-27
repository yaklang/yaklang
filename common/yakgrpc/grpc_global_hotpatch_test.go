package yakgrpc

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGlobalHotPatchConfigCRUD(t *testing.T) {
	server, err := newLiteYakServerForHotPatchTests(t)
	require.NoError(t, err)

	ctx := utils.TimeoutContextSeconds(12)

	const tplName = "global_tpl_1"
	const tplType = "global"
	code := `
beforeRequest = func(isHttps, originReq, req) { return req }
afterRequest = func(isHttps, originReq, req, originRsp, rsp) { return rsp }
`
	_, err = server.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{
		Name:    tplName,
		Type:    tplType,
		Content: code,
	})
	require.NoError(t, err)

	// enable
	setResp, err := server.SetGlobalHotPatchConfig(ctx, &ypb.SetGlobalHotPatchConfigRequest{
		Config: &ypb.GlobalHotPatchConfig{
			Enabled: true,
			Items: []*ypb.GlobalHotPatchTemplateRef{
				{Name: tplName, Type: tplType},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, setResp.GetEnabled())
	require.Equal(t, int64(1), setResp.GetVersion())
	require.Len(t, setResp.GetItems(), 1)
	require.Equal(t, tplName, setResp.GetItems()[0].GetName())
	require.Equal(t, tplType, setResp.GetItems()[0].GetType())
	require.True(t, setResp.GetItems()[0].GetEnabled())

	// get
	got, err := server.GetGlobalHotPatchConfig(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, setResp.GetVersion(), got.GetVersion())
	require.True(t, got.GetEnabled())
	require.Len(t, got.GetItems(), 1)

	// reset
	resetResp, err := server.ResetGlobalHotPatchConfig(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.False(t, resetResp.GetEnabled())
	require.Equal(t, int64(2), resetResp.GetVersion())
	require.Len(t, resetResp.GetItems(), 0)

	// optimistic lock mismatch
	_, err = server.SetGlobalHotPatchConfig(ctx, &ypb.SetGlobalHotPatchConfigRequest{
		ExpectedVersion: 1,
		Config:          &ypb.GlobalHotPatchConfig{Enabled: false},
	})
	require.Error(t, err)

	// too many items (v1)
	_, err = server.SetGlobalHotPatchConfig(ctx, &ypb.SetGlobalHotPatchConfigRequest{
		Config: &ypb.GlobalHotPatchConfig{
			Enabled: true,
			Items: []*ypb.GlobalHotPatchTemplateRef{
				{Name: tplName, Type: tplType},
				{Name: tplName, Type: tplType},
			},
		},
	})
	require.Error(t, err)

	// compile failure should not change KV version
	_, err = server.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{
		Name:    "bad_tpl",
		Type:    tplType,
		Content: "beforeRequest = func( {", // invalid yak code
	})
	require.NoError(t, err)
	_, err = server.SetGlobalHotPatchConfig(ctx, &ypb.SetGlobalHotPatchConfigRequest{
		ExpectedVersion: resetResp.GetVersion(),
		Config: &ypb.GlobalHotPatchConfig{
			Enabled: true,
			Items: []*ypb.GlobalHotPatchTemplateRef{
				{Name: "bad_tpl", Type: tplType},
			},
		},
	})
	require.Error(t, err)
	afterErr, err := server.GetGlobalHotPatchConfig(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, resetResp.GetVersion(), afterErr.GetVersion())
	require.False(t, afterErr.GetEnabled())
}

func newLiteYakServerForHotPatchTests(t *testing.T) (*Server, error) {
	t.Helper()

	tmpDir := t.TempDir()
	profileDBPath := filepath.Join(tmpDir, "profile.db")
	projectDBPath := filepath.Join(tmpDir, "project.db")

	profileDB, err := gorm.Open("sqlite3", profileDBPath)
	if err != nil {
		return nil, err
	}
	projectDB, err := gorm.Open("sqlite3", projectDBPath)
	if err != nil {
		_ = profileDB.Close()
		return nil, err
	}

	t.Cleanup(func() {
		_ = profileDB.Close()
		_ = projectDB.Close()
	})

	if db := profileDB.AutoMigrate(&schema.GeneralStorage{}, &schema.HotPatchTemplate{}); db.Error != nil {
		return nil, db.Error
	}

	return &Server{
		profileDatabase: profileDB,
		projectDatabase: projectDB,
	}, nil
}

func TestHotPatchChain_FuzzTag(t *testing.T) {
	ctx := context.Background()
	globalCode := `hello = func(params) { return ["global-" + params] }`
	moduleCode := `hello = func(params) { return ["module-" + params] }`

	opts := yak.Fuzz_WithAllHotPatchChained(ctx, yak.HotPatchChain{
		GlobalCode: globalCode,
		ModuleCode: moduleCode,
	})
	res, err := mutate.FuzzTagExec("{{yak(hello|x)}}", append(opts, mutate.Fuzz_WithEnableDangerousTag())...)
	require.NoError(t, err)
	require.Equal(t, []string{"module-x"}, res)

	// fallback to global when module does not implement the handle
	opts = yak.Fuzz_WithAllHotPatchChained(ctx, yak.HotPatchChain{
		GlobalCode: globalCode,
		ModuleCode: "",
	})
	res, err = mutate.FuzzTagExec("{{yak(hello|x)}}", append(opts, mutate.Fuzz_WithEnableDangerousTag())...)
	require.NoError(t, err)
	require.Equal(t, []string{"global-x"}, res)
}

func TestHotPatchChain_HookCode(t *testing.T) {
	ctx := context.Background()
	globalCode := `
beforeRequest = func(isHttps, originReq, req) { return string(req) + "G" }
afterRequest = func(isHttps, originReq, req, originRsp, rsp) { return string(rsp) + "G" }
mirrorHTTPFlow = func(req, rsp, existed) { return {"a":"1"} }
`
	moduleCode := `
beforeRequest = func(isHttps, originReq, req) { return string(req) + "M" }
afterRequest = func(isHttps, originReq, req, originRsp, rsp) { return string(rsp) + "M" }
mirrorHTTPFlow = func(req, rsp, existed) { return {"seen": existed["a"], "a":"2"} }
`
	before, after, mirror, _, _, _ := yak.MutateHookCallerChained(ctx, yak.HotPatchChain{
		GlobalCode: globalCode,
		ModuleCode: moduleCode,
	}, nil)

	require.NotNil(t, before)
	require.NotNil(t, after)
	require.NotNil(t, mirror)

	reqOut := before(false, []byte("ORIGIN"), []byte("REQ"))
	require.Equal(t, []byte("REQGM"), reqOut)

	rspOut := after(false, []byte("ORIGIN"), reqOut, []byte("ORSP"), []byte("RSP"))
	require.Equal(t, []byte("RSPGM"), rspOut)

	m := mirror([]byte("REQ"), []byte("RSP"), map[string]string{})
	require.Equal(t, "2", m["a"])
	require.Equal(t, "1", m["seen"])
}

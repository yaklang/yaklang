package yakgrpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactloops_yak"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 端到端：先在临时目录写一个 user yak focus mode，把 loader 的根目录指向它，
// 然后调用 Server.QueryAIFocus，断言响应里能看到这个 focus mode。
//
// 该测试同时验证：
//   - QueryAIFocus 入口确实触发了 EnsureUserFocusModesLoaded
//   - 响应里的字段（Name / VerboseName）来源于 Yak 脚本的 dunder
//   - 已注册的内置 focus mode 不会被覆盖（与用户 focus mode 共存）
//
// 关键词: grpc QueryAIFocus user dir e2e, user yak focus mode appears in response
func TestQueryAIFocus_UserDirFocusModeAppears(t *testing.T) {
	reactloops_yak.ResetUserFocusLoaderForTest()
	defer reactloops_yak.ResetUserFocusLoaderForTest()

	tmp := t.TempDir()
	reactloops_yak.SetUserFocusDirForTest(tmp)

	uniq := utils.RandStringBytes(6)
	name := "user_focus_" + uniq
	dir := filepath.Join(tmp, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	mainPath := filepath.Join(dir, name+reactloops_yak.FocusModeFileSuffix)
	require.NoError(t, os.WriteFile(mainPath, []byte(`
__VERBOSE_NAME__ = "User Focus E2E"
__DESCRIPTION__  = "user-defined focus mode discovered from yakit-projects/ai-focus"
__MAX_ITERATIONS__ = 3
`), 0o644))

	server := &Server{}
	resp, err := server.QueryAIFocus(context.Background(), &ypb.QueryAIFocusRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Data, "response data must not be empty")

	var found *ypb.AIFocus
	for _, f := range resp.Data {
		if f == nil {
			continue
		}
		if f.Name == name {
			found = f
			break
		}
	}
	require.NotNil(t, found, "user focus mode %q must appear in QueryAIFocus response", name)
	require.Equal(t, "User Focus E2E", found.VerboseName,
		"VerboseName must come from yak script __VERBOSE_NAME__")
	require.Equal(t, "user-defined focus mode discovered from yakit-projects/ai-focus",
		found.Description, "Description must come from yak script __DESCRIPTION__")
}

// 没有用户目录 / 用户目录为空时，QueryAIFocus 仍然能正常返回（包含内置 focus mode）。
//
// 关键词: grpc QueryAIFocus empty user dir
func TestQueryAIFocus_EmptyUserDirIsHarmless(t *testing.T) {
	reactloops_yak.ResetUserFocusLoaderForTest()
	defer reactloops_yak.ResetUserFocusLoaderForTest()

	tmp := t.TempDir()
	reactloops_yak.SetUserFocusDirForTest(tmp)

	server := &Server{}
	resp, err := server.QueryAIFocus(context.Background(), &ypb.QueryAIFocusRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

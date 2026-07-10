package yakgrpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func setupNotifyTestDB(t *testing.T) {
	t.Helper()
	oldDB := consts.GetGormProfileDatabase()
	oldPath := consts.GetCurrentProfileDatabasePath()

	dbPath := filepath.Join(t.TempDir(), "profile.db")
	db, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create profile db: %v", err)
	}
	consts.BindProfileDatabase(db, dbPath)
	t.Cleanup(func() {
		consts.BindProfileDatabase(oldDB, oldPath)
		_ = db.Close()
	})
}

func TestSetupNotifyTestDBRestoresProfileDatabase(t *testing.T) {
	oldDB := consts.GetGormProfileDatabase()
	oldPath := consts.GetCurrentProfileDatabasePath()

	t.Run("scoped bind", func(t *testing.T) {
		setupNotifyTestDB(t)
		if consts.GetGormProfileDatabase() == oldDB {
			t.Fatal("setupNotifyTestDB did not bind a temporary profile DB")
		}
	})

	if got := consts.GetGormProfileDatabase(); got != oldDB {
		t.Fatal("setupNotifyTestDB leaked temporary profile DB after test cleanup")
	}
	if got := consts.GetCurrentProfileDatabasePath(); got != oldPath {
		t.Fatalf("profile db path leaked: got %q, want %q", got, oldPath)
	}
}

func TestServer_SaveListDeleteIMBot(t *testing.T) {
	setupNotifyTestDB(t)
	s := &Server{}
	ctx := context.Background()

	// 飞书
	resp, err := s.SaveIMBot(ctx, &ypb.SaveIMBotRequest{Bot: &ypb.IMBotConfig{
		Platform: notify.PlatformFeishu.String(), AppId: "cli_f", AppSecret: "sf", Enabled: true,
	}})
	if err != nil {
		t.Fatalf("save feishu: %v", err)
	}
	if resp.GetBot().GetAppId() != "cli_f" {
		t.Fatalf("returned bot = %+v", resp.GetBot())
	}

	// 钉钉
	if _, err := s.SaveIMBot(ctx, &ypb.SaveIMBotRequest{Bot: &ypb.IMBotConfig{
		Platform: notify.PlatformDingTalk.String(), AppId: "dk", AppSecret: "sd", RobotSecret: "rs",
	}}); err != nil {
		t.Fatalf("save dingtalk: %v", err)
	}

	// 列表
	list, err := s.ListIMBots(ctx, &ypb.ListIMBotRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.GetBots()) != 2 {
		t.Fatalf("expected 2 bots, got %d", len(list.GetBots()))
	}

	// 删除飞书
	if _, err := s.DeleteIMBot(ctx, &ypb.DeleteIMBotRequest{Platform: notify.PlatformFeishu.String()}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = s.ListIMBots(ctx, &ypb.ListIMBotRequest{})
	if len(list.GetBots()) != 1 || list.GetBots()[0].GetPlatform() != notify.PlatformDingTalk.String() {
		t.Fatalf("expected only dingtalk after delete, got %+v", list.GetBots())
	}
}

func TestServer_SaveIMBot_Validation(t *testing.T) {
	setupNotifyTestDB(t)
	s := &Server{}
	ctx := context.Background()
	if _, err := s.SaveIMBot(ctx, &ypb.SaveIMBotRequest{}); err == nil {
		t.Fatal("nil bot should error")
	}
	if _, err := s.SaveIMBot(ctx, &ypb.SaveIMBotRequest{Bot: &ypb.IMBotConfig{Platform: "bogus"}}); err == nil {
		t.Fatal("bogus platform should error")
	}
}

func TestServer_SaveIMBot_PreservesOwnerID(t *testing.T) {
	setupNotifyTestDB(t)
	srv, _ := newIMBotWelcomeServer(t)
	t.Cleanup(srv.Close)
	s := &Server{}
	ctx := context.Background()

	resp, err := s.SaveIMBot(ctx, &ypb.SaveIMBotRequest{Bot: &ypb.IMBotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppId:     "cli_owner",
		AppSecret: "sec_owner",
		BaseUrl:   srv.URL,
		Enabled:   true,
		OwnerId:   "ou_owner",
	}})
	if err != nil {
		t.Fatalf("save bot: %v", err)
	}
	if resp.GetBot().GetOwnerId() != "ou_owner" {
		t.Fatalf("saved OwnerId = %q, want ou_owner", resp.GetBot().GetOwnerId())
	}

	list, err := s.ListIMBots(ctx, &ypb.ListIMBotRequest{})
	if err != nil {
		t.Fatalf("list bots: %v", err)
	}
	if len(list.GetBots()) != 1 || list.GetBots()[0].GetOwnerId() != "ou_owner" {
		t.Fatalf("listed bots = %+v, want owner ou_owner", list.GetBots())
	}
}

func TestServer_SaveIMBot_DefersWelcomeUntilIMControlStarts(t *testing.T) {
	setupNotifyTestDB(t)
	srv, sent := newIMBotWelcomeServer(t)
	t.Cleanup(srv.Close)
	s := &Server{}
	ctx := context.Background()
	imEngineMu.Lock()
	s.imEngine = nil
	pendingIMOnboardingWelcomes = map[string]*credential.BotConfig{}
	imEngineMu.Unlock()

	if _, err := s.SaveIMBot(ctx, &ypb.SaveIMBotRequest{Bot: &ypb.IMBotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppId:     "cli_welcome",
		AppSecret: "sec_welcome",
		BaseUrl:   srv.URL,
		Enabled:   true,
		OwnerId:   "ou_owner",
	}}); err != nil {
		t.Fatalf("save bot: %v", err)
	}
	if len(*sent) != 0 {
		t.Fatalf("welcome sends = %d, want deferred until IM control starts", len(*sent))
	}
	imEngineMu.Lock()
	pending := pendingIMOnboardingWelcomes[notify.PlatformFeishu.String()]
	delete(pendingIMOnboardingWelcomes, notify.PlatformFeishu.String())
	imEngineMu.Unlock()
	if pending == nil || pending.OwnerID != "ou_owner" {
		t.Fatalf("pending welcome = %+v, want owner ou_owner", pending)
	}
}

func newIMBotWelcomeServer(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var sent []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-token","expire":7200}`))
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/im/v1/messages":
			if got := r.URL.Query().Get("receive_id_type"); got != "open_id" {
				t.Fatalf("receive_id_type = %q, want open_id", got)
			}
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &body)
			}
			sent = append(sent, body)
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om_welcome"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"msg":"not found"}`))
		}
	}))
	return srv, &sent
}

func TestOnboardingStepToPBIncludesOwnerID(t *testing.T) {
	step := &notify.OnboardingStep{
		State:   "success",
		QrURL:   "qr-url",
		QrPNG:   []byte("png-bytes"),
		Message: "ok",
		Result: &notify.OnboardingResult{
			Platform:  notify.PlatformFeishu,
			AppID:     "cli_owner",
			AppSecret: "sec_owner",
			OwnerID:   "ou_owner",
		},
	}

	ev := onboardingStepToPB(step)
	if ev.GetBot().GetOwnerId() != "ou_owner" {
		t.Fatalf("OwnerId = %q, want ou_owner", ev.GetBot().GetOwnerId())
	}
	if ev.GetQrImageBase64() != base64.StdEncoding.EncodeToString([]byte("png-bytes")) {
		t.Fatalf("QrImageBase64 = %q", ev.GetQrImageBase64())
	}
}

func TestServer_TestIMBot_InvalidCreds(t *testing.T) {
	// 用明显无效的凭证 + mock 不可能到达，应返回 Ok=false（凭证校验失败）。
	// 这里不连真实平台：bad host 通过 BaseURL 指向不可达地址快速失败。
	setupNotifyTestDB(t)
	s := &Server{}
	ctx := context.Background()
	resp, err := s.TestIMBot(ctx, &ypb.TestIMBotRequest{
		Bot: &ypb.IMBotConfig{
			Platform:  notify.PlatformFeishu.String(),
			AppId:     "x",
			AppSecret: "x",
			BaseUrl:   "http://127.0.0.1:1", // 不可达端口，快速失败
		},
	})
	if err != nil {
		t.Fatalf("test should not return transport error: %v", err)
	}
	if resp.GetOk() {
		t.Fatal("expected Ok=false for invalid/unreachable creds")
	}
	if resp.GetMessage() == "" {
		t.Fatal("expected error message")
	}
}

package credential

import (
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/notify"
)

// setupTestDB 绑定一个临时 profile 库，避免污染真实数据。
func setupTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "profile.db")
	db, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create profile db: %v", err)
	}
	consts.BindProfileDatabase(db, dbPath)
}

func TestEncryptDecryptSecret_RoundTrip(t *testing.T) {
	cases := []string{
		"",
		"cli_xxx",
		"SECabcdef123456",
		"含中文的 secret 🔑",
	}
	for _, plain := range cases {
		enc, err := encryptSecret(plain)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plain, err)
		}
		if plain == "" {
			if enc != "" {
				t.Fatalf("empty should stay empty, got %q", enc)
			}
			continue
		}
		// 密文不应等于明文
		if enc == plain {
			t.Fatalf("secret %q was not encrypted", plain)
		}
		got, err := decryptSecret(enc)
		if err != nil {
			t.Fatalf("decrypt %q: %v", plain, err)
		}
		if got != plain {
			t.Fatalf("round-trip mismatch: want %q got %q", plain, got)
		}
	}
}

func TestDecryptSecret_FallbackPlaintext(t *testing.T) {
	// 历史明文（非 hex）应原样返回，不报错。
	got, err := decryptSecret("plaintext-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plaintext-secret" {
		t.Fatalf("want plaintext fallback, got %q", got)
	}
}

func TestSaveAndGetBotConfig(t *testing.T) {
	setupTestDB(t)

	cfg := &BotConfig{
		Platform:    notify.PlatformFeishu.String(),
		AppID:       "cli_test",
		AppSecret:   "secret-feishu",
		RobotSecret: "",
		BaseURL:     "",
		Enabled:     true,
	}
	saved, err := SaveBotConfig(cfg)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.AppSecret != "secret-feishu" {
		t.Fatalf("returned secret not plaintext: %q", saved.AppSecret)
	}

	got, err := GetBotConfig(notify.PlatformFeishu.String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.AppID != "cli_test" || got.AppSecret != "secret-feishu" {
		t.Fatalf("got = %+v", got)
	}
	// 确认落库的是密文
	var raw BotConfig
	dbRow := getDB().Where("platform = ?", got.Platform).First(&raw)
	if dbRow.Error != nil || raw.AppSecret == "secret-feishu" {
		t.Fatalf("secret stored in plaintext: %q (err=%v)", raw.AppSecret, dbRow.Error)
	}
}

func TestSaveBotConfig_Upsert(t *testing.T) {
	setupTestDB(t)

	p := notify.PlatformDingTalk.String()
	_, err := SaveBotConfig(&BotConfig{Platform: p, AppID: "k1", AppSecret: "s1", RobotSecret: "rs1"})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	updated, err := SaveBotConfig(&BotConfig{Platform: p, AppID: "k2", AppSecret: "s2", RobotSecret: "rs2"})
	if err != nil {
		t.Fatalf("upsert save: %v", err)
	}
	got, err := GetBotConfig(p)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AppID != "k2" || got.AppSecret != "s2" || got.RobotSecret != "rs2" {
		t.Fatalf("upsert not applied: %+v", got)
	}
	// 应仍只有一条
	list, _ := ListBotConfigs()
	if len(list) != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", len(list))
	}
	_ = updated
}

// TestSaveBotConfig_PreservesPermissionFields 验证 upsert 时不传权限字段会保留旧值，
// 传了新值会覆盖。
func TestSaveBotConfig_PreservesPermissionFields(t *testing.T) {
	setupTestDB(t)

	p := notify.PlatformFeishu.String()
	// 第一次：写入带权限字段
	_, err := SaveBotConfig(&BotConfig{
		Platform: p, AppID: "a1", AppSecret: "s1",
		OwnerID: "ou_owner", AllowedUsers: `["ou_a","ou_b"]`, AllowedChats: `["oc_x"]`,
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	// 第二次：只更新凭证，不传权限字段（空）→ 应保留旧权限
	_, err = SaveBotConfig(&BotConfig{Platform: p, AppID: "a2", AppSecret: "s2"})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, err := GetBotConfig(p)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AppID != "a2" {
		t.Errorf("AppID = %q, want a2", got.AppID)
	}
	if got.OwnerID != "ou_owner" {
		t.Errorf("OwnerID should be preserved, got %q", got.OwnerID)
	}
	if got.AllowedUsers != `["ou_a","ou_b"]` {
		t.Errorf("AllowedUsers should be preserved, got %q", got.AllowedUsers)
	}
	if got.AllowedChats != `["oc_x"]` {
		t.Errorf("AllowedChats should be preserved, got %q", got.AllowedChats)
	}

	// 第三次：显式传新 OwnerID → 应覆盖
	_, err = SaveBotConfig(&BotConfig{Platform: p, AppID: "a3", AppSecret: "s3", OwnerID: "ou_new_owner"})
	if err != nil {
		t.Fatalf("third save: %v", err)
	}
	got2, _ := GetBotConfig(p)
	if got2.OwnerID != "ou_new_owner" {
		t.Errorf("OwnerID should be updated to ou_new_owner, got %q", got2.OwnerID)
	}
}

func TestListAndDeleteBotConfig(t *testing.T) {
	setupTestDB(t)
	_, _ = SaveBotConfig(&BotConfig{Platform: notify.PlatformFeishu.String(), AppID: "f", AppSecret: "sf"})
	_, _ = SaveBotConfig(&BotConfig{Platform: notify.PlatformDingTalk.String(), AppID: "d", AppSecret: "sd", RobotSecret: "r"})

	list, err := ListBotConfigs()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(list))
	}
	// 解密后应可读
	for _, c := range list {
		if c.AppSecret == "" {
			t.Fatalf("secret not decrypted for %s", c.Platform)
		}
	}

	if err := DeleteBotConfig(notify.PlatformFeishu.String()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := GetBotConfig(notify.PlatformFeishu.String())
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}

func TestSaveBotConfig_Validation(t *testing.T) {
	setupTestDB(t)
	if _, err := SaveBotConfig(nil); err == nil {
		t.Fatal("nil should error")
	}
	if _, err := SaveBotConfig(&BotConfig{Platform: ""}); err == nil {
		t.Fatal("empty platform should error")
	}
	if _, err := SaveBotConfig(&BotConfig{Platform: "nonsense"}); err == nil {
		t.Fatal("unsupported platform should error")
	}
}

func TestBotConfig_ToSendConfig(t *testing.T) {
	b := &BotConfig{AppID: "a", AppSecret: "s", RobotSecret: "r", BaseURL: "u"}
	cfg := b.ToSendConfig()
	if cfg.AppID != "a" || cfg.AppSecret != "s" || cfg.RobotSecret != "r" || cfg.BaseURL != "u" {
		t.Fatalf("send config mismatch: %+v", cfg)
	}
	if b.ToSendConfig() == nil {
		t.Fatal("nil bot should return empty config, not nil value pointer issue")
	}
}

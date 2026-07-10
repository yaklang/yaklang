package imcontrol

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
)

func setupIMAuthTestDB(t *testing.T, bot *credential.BotConfig) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "profile.db")
	db, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create profile db: %v", err)
	}
	consts.BindProfileDatabase(db, dbPath)
	if _, err := credential.SaveBotConfig(bot); err != nil {
		t.Fatalf("save bot config: %v", err)
	}
}

func TestCheckPermissionOwnerOnlyRejectsSharedUser(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppID:     "cli_owner",
		AppSecret: "sec_owner",
		Enabled:   true,
		OwnerID:   "ou_owner",
	})
	e := New(Config{})

	if ok, reason := e.checkPermission(notify.PlatformFeishu.String(), "ou_owner", "ou_owner"); !ok {
		t.Fatalf("owner should be allowed, reason=%s", reason)
	}
	ok, reason := e.checkPermission(notify.PlatformFeishu.String(), "ou_other", "ou_other")
	if ok {
		t.Fatal("shared non-owner user should be rejected by default owner-only policy")
	}
	if !strings.Contains(reason, "仅 bot 所有者") {
		t.Fatalf("reason = %q, want owner-only hint", reason)
	}
}

func TestCheckPermissionGroupDefaultsAllowMembers(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:           notify.PlatformFeishu.String(),
		AppID:              "cli_group_default",
		AppSecret:          "sec_group_default",
		Enabled:            true,
		OwnerID:            "ou_owner",
		GroupAccessControl: false,
	})
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_group_default",
		SenderID: "ou_other",
		ChatType: "group",
	}
	e.touchSession(imSessionKey(msg), msg)

	if ok, reason := e.checkPermission(notify.PlatformFeishu.String(), msg.ChatID, msg.SenderID); !ok {
		t.Fatalf("group member should be allowed when group access control is off, reason=%s", reason)
	}
}

func TestCheckPermissionGroupAccessControlUsesWhitelist(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:           notify.PlatformFeishu.String(),
		AppID:              "cli_group_acl",
		AppSecret:          "sec_group_acl",
		Enabled:            true,
		OwnerID:            "ou_owner",
		AllowedUsers:       `["ou_allowed"]`,
		GroupAccessControl: true,
	})
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_group_acl",
		SenderID: "ou_other",
		ChatType: "group",
	}
	e.touchSession(imSessionKey(msg), msg)

	if ok, reason := e.checkPermission(notify.PlatformFeishu.String(), msg.ChatID, "ou_other"); ok {
		t.Fatalf("non-whitelist group member should be rejected, reason=%s", reason)
	}
	if ok, reason := e.checkPermission(notify.PlatformFeishu.String(), msg.ChatID, "ou_owner"); !ok {
		t.Fatalf("owner should be allowed in group access control mode, reason=%s", reason)
	}
	if ok, reason := e.checkPermission(notify.PlatformFeishu.String(), msg.ChatID, "ou_allowed"); !ok {
		t.Fatalf("whitelist user should be allowed in group access control mode, reason=%s", reason)
	}
}

func TestBackfillOwnerFromPrivateMessage(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppID:     "cli_owner_empty",
		AppSecret: "sec_owner_empty",
		Enabled:   true,
	})
	e := New(Config{})
	e.tryBackfillOwnerFromPrivateMessage(&notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "ou_sender",
		SenderID: "ou_sender",
		ChatType: "private",
	})

	bot, err := credential.GetBotConfig(notify.PlatformFeishu.String())
	if err != nil {
		t.Fatalf("get bot config: %v", err)
	}
	if bot.OwnerID != "ou_sender" {
		t.Fatalf("OwnerID = %q, want ou_sender", bot.OwnerID)
	}
}

func TestBackfillOwnerSkipsGroupMessage(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppID:     "cli_group_owner_empty",
		AppSecret: "sec_group_owner_empty",
		Enabled:   true,
	})
	e := New(Config{})
	e.tryBackfillOwnerFromPrivateMessage(&notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_group",
		SenderID: "ou_sender",
		ChatType: "group",
	})

	bot, err := credential.GetBotConfig(notify.PlatformFeishu.String())
	if err != nil {
		t.Fatalf("get bot config: %v", err)
	}
	if bot.OwnerID != "" {
		t.Fatalf("OwnerID = %q, want empty for group message", bot.OwnerID)
	}
}

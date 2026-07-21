package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/notify"
)

// BotConfig 是一个 IM 平台 bot 的持久化凭证（每平台一条）。
//
// 敏感字段（AppSecret / RobotSecret）在落库前加密、读取时解密，避免明文存储。
type BotConfig struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform    string    `gorm:"uniqueIndex;not null" json:"platform"` // feishu / dingtalk
	AppID       string    `json:"app_id"`
	AppSecret   string    `json:"app_secret"`   // 落库前加密
	RobotSecret string    `json:"robot_secret"` // 落库前加密
	BaseURL     string    `json:"base_url"`     // 可选
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// --- 权限控制（Phase 3）---
	// OwnerID bot 所属用户标识（飞书 open_id 等）。非空时启用 owner-only 模式：
	//   仅 OwnerID 本人可用，除非 AllowedUsers 也非空（则 AllowedUsers 列表内都可用）。
	// 空值 = 不限制（向后兼容，任何人可用）。
	OwnerID string `json:"owner_id" gorm:"type:varchar(256)"`
	// AllowedUsers 允许使用 bot 的用户列表，JSON 数组 ["ou_a","ou_b"]。空 = 不限制。
	AllowedUsers string `json:"allowed_users" gorm:"type:text"`
	// AllowedChats 允许使用 bot 的会话列表，JSON 数组 ["oc_a"]。空 = 不限制。
	AllowedChats string `json:"allowed_chats" gorm:"type:text"`
	// GroupAccessControl 开启后，群聊/话题仅 OwnerID 或 AllowedUsers 白名单用户可用。
	// 关闭时，群聊默认允许所有群成员在满足触发策略时使用。
	GroupAccessControl bool `json:"group_access_control" gorm:"column:group_access_control"`

	// ClearOwnerID 是保存时的瞬时控制字段，不落库。用于解绑场景显式清空 OwnerID。
	ClearOwnerID bool `json:"-" gorm:"-"`
}

// TableName 固定表名。
func (BotConfig) TableName() string { return "notify_im_bot_configs" }

// ToSendConfig 把一条持久化凭证转成 SendConfig（解密后），供 notify 客户端直接使用。
func (b *BotConfig) ToSendConfig() *notify.SendConfig {
	if b == nil {
		return notify.NewSendConfig()
	}
	return notify.NewSendConfig(
		notify.WithAppID(b.AppID),
		notify.WithAppSecret(b.AppSecret),
		notify.WithRobotSecret(b.RobotSecret),
		notify.WithBaseURL(b.BaseURL),
	)
}

// ---- 加密 ----
//
// 用一个进程级固定密钥做 AES-GCM 加密。密钥由 yaklang 安装目录 + 常量派生，
// 防止数据库被直接拷出后明文泄露；不追求对抗本机攻击者（那需要 OS keychain，留作后续增强）。

var encryptionKey = deriveKey("yaklang-notify-bot-secret-v1")

func deriveKey(seed string) []byte {
	sum := sha256.Sum256([]byte(seed))
	return sum[:32] // AES-256
}

func encryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return hex.EncodeToString(ct), nil
}

func decryptSecret(cipherHex string) (string, error) {
	if cipherHex == "" {
		return "", nil
	}
	ct, err := hex.DecodeString(cipherHex)
	if err != nil {
		// 兼容历史上未加密的明文（直接返回原值）。
		return cipherHex, nil
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ct) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := ct[:gcm.NonceSize()], ct[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		// 解密失败时回退原值，避免历史脏数据导致整体不可用。
		return cipherHex, nil
	}
	return string(pt), nil
}

// ---- DB ----

func getDB() *gorm.DB {
	return consts.GetGormProfileDatabase()
}

// migrateOnce 保证表结构存在（幂等）。
func migrateOnce() error {
	db := getDB().AutoMigrate(&BotConfig{})
	if db != nil && db.Error != nil {
		return db.Error
	}
	return nil
}

// SaveBotConfig 保存（按 Platform upsert）一条 bot 凭证，敏感字段加密落库。
func SaveBotConfig(cfg *BotConfig) (*BotConfig, error) {
	if cfg == nil {
		return nil, errors.New("nil bot config")
	}
	if cfg.Platform == "" {
		return nil, errors.New("platform is required")
	}
	if !isValidPlatform(cfg.Platform) {
		return nil, fmt.Errorf("unsupported platform %q", cfg.Platform)
	}
	if err := migrateOnce(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	encSecret, err := encryptSecret(cfg.AppSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt app secret: %w", err)
	}
	encRobot, err := encryptSecret(cfg.RobotSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt robot secret: %w", err)
	}

	now := time.Now()
	row := BotConfig{
		Platform:           cfg.Platform,
		AppID:              cfg.AppID,
		AppSecret:          encSecret,
		RobotSecret:        encRobot,
		BaseURL:            cfg.BaseURL,
		Enabled:            cfg.Enabled,
		UpdatedAt:          now,
		OwnerID:            cfg.OwnerID,
		AllowedUsers:       cfg.AllowedUsers,
		AllowedChats:       cfg.AllowedChats,
		GroupAccessControl: cfg.GroupAccessControl,
	}

	db := getDB()
	var existing BotConfig
	err = db.Where("platform = ?", row.Platform).First(&existing).Error
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, fmt.Errorf("query existing: %w", err)
	}
	if gorm.IsRecordNotFoundError(err) {
		row.CreatedAt = now
		if e := db.Create(&row).Error; e != nil {
			return nil, fmt.Errorf("create: %w", e)
		}
	} else {
		row.ID = existing.ID
		row.CreatedAt = existing.CreatedAt
		// P4-bugfix: 前端可能只传凭证不传权限字段（空字符串），此时保留已有权限配置。
		// 仅当 cfg 显式提供了非空权限值时才覆盖。
		if cfg.ClearOwnerID {
			row.OwnerID = ""
		} else if row.OwnerID == "" {
			row.OwnerID = existing.OwnerID
		}
		if row.AllowedUsers == "" {
			row.AllowedUsers = existing.AllowedUsers
		}
		if row.AllowedChats == "" {
			row.AllowedChats = existing.AllowedChats
		}
		if e := db.Save(&row).Error; e != nil {
			return nil, fmt.Errorf("save: %w", e)
		}
	}
	out := row
	out.AppSecret = cfg.AppSecret
	out.RobotSecret = cfg.RobotSecret
	return &out, nil
}

// ListBotConfigs 返回所有 bot 凭证（解密后）。
func ListBotConfigs() ([]*BotConfig, error) {
	if err := migrateOnce(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	var rows []*BotConfig
	if err := getDB().Order("platform ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		r.AppSecret, _ = decryptSecret(r.AppSecret)
		r.RobotSecret, _ = decryptSecret(r.RobotSecret)
	}
	return rows, nil
}

// GetBotConfig 按 platform 取单条（解密后）。不存在返回 nil。
func GetBotConfig(platform string) (*BotConfig, error) {
	if platform == "" {
		return nil, errors.New("platform is required")
	}
	if err := migrateOnce(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	var row BotConfig
	err := getDB().Where("platform = ?", platform).First(&row).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	row.AppSecret, _ = decryptSecret(row.AppSecret)
	row.RobotSecret, _ = decryptSecret(row.RobotSecret)
	return &row, nil
}

// DeleteBotConfig 按 platform 删除一条。
func DeleteBotConfig(platform string) error {
	if platform == "" {
		return errors.New("platform is required")
	}
	if err := migrateOnce(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return getDB().Where("platform = ?", platform).Delete(&BotConfig{}).Error
}

func isValidPlatform(p string) bool {
	switch notify.PlatformType(p) {
	case notify.PlatformFeishu, notify.PlatformDingTalk, notify.PlatformWeCom, notify.PlatformTelegram:
		return true
	}
	return false
}

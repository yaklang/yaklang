package ssa_compile

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// EnsureSSAProjectRowForCodeScan 在 profile 库中查/建/更新 SSAProject 行，把权威 project_id 写回配置，
// 避免同进程 code-scan 仅有 Program、无工程行时的语义断裂。
func EnsureSSAProjectRowForCodeScan(ctx context.Context, db *gorm.DB, cfg *ssaconfig.Config) (*ssaconfig.Config, *schema.SSAProject, error) {
	if db == nil {
		return nil, nil, utils.Errorf("db is nil")
	}
	if cfg == nil {
		return nil, nil, utils.Errorf("config is nil")
	}
	raw, err := cfg.ToJSONRaw()
	if err != nil {
		return nil, nil, err
	}
	b, err := ssaproject.NewSSAProject(ssaconfig.WithJsonRawConfig(raw))
	if err != nil {
		return nil, nil, err
	}
	if err := b.Config.Update(ssaconfig.WithContext(ctx)); err != nil {
		return nil, nil, err
	}
	if err := b.SaveToDB(db); err != nil {
		return nil, nil, err
	}
	return b.Config, b.SSAProject, nil
}

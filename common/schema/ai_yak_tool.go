package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AIYakTool struct {
	gorm.Model

	Name string `json:"name" gorm:"unique_index"`
	VerboseName   string `json:"verbose_name"`    // English string；列表双语另见 VerboseNameToI18n / VerboseNameI18n
	VerboseNameZh string `json:"verbose_name_zh"` // Chinese storage；勿把 gRPC VerboseName 改成 I18n 对象
	Description   string `json:"description" gorm:"type:text;index"`
	Keywords    string `json:"keywords" gorm:"type:text;index"`
	// 工具使用说明，在参数生成阶段披露给 AI（2阶段披露），帮助 AI 更好地使用参数
	Usage      string `json:"usage" gorm:"type:text"`
	Content    string `json:"content" gorm:"type:text"`
	Params     string `json:"params" gorm:"type:text"`
	Path       string `json:"path" gorm:"type:text;index"`
	Author     string `json:"author"`
	IsBuiltin  bool   `json:"is_builtin" gorm:"default:false;index"`
	Hash       string `json:"hash"`
	IsFavorite bool   `json:"is_favorite" gorm:"default:false;index"`
	// 0: unset, 1: disabled, 2: enabled
	EnableAIOutputLog int `json:"enable_ai_output_log" gorm:"default:0"`
}

func (a *AIYakTool) ToUpdateMap() map[string]interface{} {
	if a == nil {
		return nil
	}

	return map[string]interface{}{
		"name":                 a.Name,
		"verbose_name":         a.VerboseName,
		"verbose_name_zh":      a.VerboseNameZh,
		"description":          a.Description,
		"keywords":             a.Keywords,
		"usage":                a.Usage,
		"content":              a.Content,
		"params":               a.Params,
		"path":                 a.Path,
		"is_builtin":           a.IsBuiltin,
		"hash":                 a.CalcHash(),
		"is_favorite":          a.IsFavorite,
		"enable_ai_output_log": a.EnableAIOutputLog,
	}
}

// VerboseNameToI18n builds AIOutputI18n-compatible Zh/En for list / event UIs.
func (a *AIYakTool) VerboseNameToI18n() *I18n {
	if a == nil {
		return nil
	}
	return NewI18n(a.VerboseNameZh, a.VerboseName)
}

func (a *AIYakTool) ToGRPC() *ypb.AITool {
	out := &ypb.AITool{
		Name:        a.Name,
		Description: a.Description,
		Content:     a.Content,
		ToolPath:    a.Path,
		Keywords:    utils.PrettifyListFromStringSplitEx(a.Keywords, ",", "|"),
		IsFavorite:  a.IsFavorite,
		ID:          int64(a.ID),
		VerboseName: a.VerboseName,
		Author:      a.Author,
		IsBuiltin:   a.IsBuiltin,
		CreatedAt:   a.CreatedAt.Unix(),
		UpdatedAt:   a.UpdatedAt.Unix(),
	}
	if i18n := a.VerboseNameToI18n(); i18n != nil {
		out.VerboseNameI18N = i18n.I18nToYPB_I18n()
	}
	return out
}

func (*AIYakTool) TableName() string {
	return "ai_yak_tools"
}

func (d *AIYakTool) CalcHash() string {
	return utils.CalcSha1(d.Name, d.Content, d.Params, d.Path, d.Description, d.Keywords, d.Usage)
}

func (d *AIYakTool) BeforeCreate() error {
	d.Author = NormalizeAIResourceAuthor(d.Author, AIResourceAuthorAnonymous)
	return nil
}

func (d *AIYakTool) BeforeSave() error {
	d.Hash = d.CalcHash()
	return nil
}

package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AIYakTool struct {
	gorm.Model

	Name string `json:"name" gorm:"unique_index"`
	// 展示给用户的名称
	VerboseName string `json:"verbose_name"`
	Description string `json:"description" gorm:"type:text;index"`
	Keywords    string `json:"keywords" gorm:"type:text;index"`
	// 工具使用说明，在参数生成阶段披露给 AI（2阶段披露），帮助 AI 更好地使用参数
	Usage      string `json:"usage" gorm:"type:text"`
	Content    string `json:"content" gorm:"type:text"`
	Params     string `json:"params" gorm:"type:text"`
	Path       string `json:"path" gorm:"type:text;index"`
	Author     string `json:"author"`
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
		"description":          a.Description,
		"keywords":             a.Keywords,
		"usage":                a.Usage,
		"content":              a.Content,
		"params":               a.Params,
		"path":                 a.Path,
		"hash":                 a.CalcHash(),
		"is_favorite":          a.IsFavorite,
		"enable_ai_output_log": a.EnableAIOutputLog,
	}
}

func (a *AIYakTool) ToGRPC() *ypb.AITool {
	return &ypb.AITool{
		Name:        a.Name,
		Description: a.Description,
		Content:     a.Content,
		ToolPath:    a.Path,
		Keywords:    utils.PrettifyListFromStringSplitEx(a.Keywords, ",", "|"),
		IsFavorite:  a.IsFavorite,
		ID:          int64(a.ID),
		VerboseName: a.VerboseName,
		Author:      a.Author,
		CreatedAt:   a.CreatedAt.Unix(),
		UpdatedAt:   a.UpdatedAt.Unix(),
	}
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

package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type WebShell struct {
	gorm.Model
	Url string `json:"url" gorm:"index" `
	// pass=payload
	Pass string `json:"pass"`
	// 加密密钥
	SecretKey string `json:"secret_key" gorm:"not null"`
	// 加密模式
	EncryptedMode string `json:"enc_mode" gorm:"column:enc_mode;not null"`
	// 字符集编码
	Charset string `json:"charset" gorm:"default:'UTF-8'"`
	// 冰蝎还是哥斯拉,或者是其他
	ShellType string `json:"shell_type"`
	// 脚本语言
	ShellScript string            `json:"shell_script"`
	Headers     map[string]string `json:"headers"`
	Status      bool              `json:"status"`
	Tag         string            `json:"tag"`
	Hash        string            `json:"hash"`
}

func (w *WebShell) CalcHash() string {
	return utils.CalcSha1(w.Url)
}

func (w *WebShell) BeforeSave() error {
	if w.Url == "" {
		return utils.Errorf("webshell url is empty")
	}
	if w.EncryptedMode == "" {
		return utils.Errorf("webshell encrypted mode is empty")
	}
	if w.ShellScript == "" {
		return utils.Errorf("webshell shell script  is empty")
	}
	w.Hash = w.CalcHash()
	return nil
}

func (w *WebShell) ToGRPCModel() *ypb.WebShell {

	return &ypb.WebShell{
		Id:          int64(w.ID),
		Url:         w.Url,
		Pass:        w.Pass,
		SecretKey:   w.SecretKey,
		EncMode:     w.EncryptedMode,
		Charset:     w.Charset,
		ShellType:   w.ShellType,
		ShellScript: w.ShellScript,
		Status:      w.Status,
		Tag:         w.Tag,
		CreatedAt:   w.CreatedAt.Unix(),
		UpdatedAt:   w.UpdatedAt.Unix(),
	}
}

func CreateOrUpdateWebShell(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&WebShell{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&WebShell{}); db.Error != nil {
		return utils.Errorf("create/update WebShell failed: %s", db.Error)
	}

	return nil
}

func CreateOrUpdateWebShellById(db *gorm.DB, id int64, i interface{}) error {
	db = db.Model(&WebShell{})

	if db := db.Where("id = ?", id).Assign(i).FirstOrCreate(&WebShell{}); db.Error != nil {
		return utils.Errorf("create/update WebShell failed: %s", db.Error)
	}

	return nil
}

func DeleteWebShellByID(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&WebShell{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&WebShell{}); db.Error != nil {
			return db.Error
		}
		return nil
	}
	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&WebShell{}); db.Error != nil {
		return utils.Errorf("delete id(s) failed: %v", db.Error)
	}
	return nil
}

func GetWebShell(db *gorm.DB, id int64) (*ypb.WebShell, error) {
	var req WebShell
	if db := db.Model(&WebShell{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get WebShell failed: %s", db.Error)
	}
	return req.ToGRPCModel(), nil
}

func QueryWebShell(db *gorm.DB, params *ypb.QueryWebShellsRequest) (*bizhelper.Paginator, []*WebShell, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}

	db = db.Model(&WebShell{}) // .Debug()
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination

	var ret []*WebShell
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

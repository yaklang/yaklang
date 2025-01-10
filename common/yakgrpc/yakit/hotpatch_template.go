package yakit

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type QueryHotPatchTemplateConfig struct {
	IDs             []int64
	Names           []string
	ContentKeyWords []string
	Type            string
}

func NewHotPatchTemplate(name, content, typ string) *schema.HotPatchTemplate {
	return &schema.HotPatchTemplate{
		Name:    name,
		Content: content,
		Type:    typ,
	}
}

func FilterHotPatchTemplateIsEmpty(filter *ypb.HotPatchTemplateRequest) bool {
	if len(filter.GetId()) == 0 && len(filter.GetName()) == 0 && len(filter.GetContentKeyword()) == 0 && filter.GetType() == "" {
		return true
	}
	return false
}

func FilterHotPatchTemplate(db *gorm.DB, filter *ypb.HotPatchTemplateRequest) *gorm.DB {
	if ids := filter.GetId(); len(ids) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids)
	}
	if names := filter.GetName(); len(names) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "name", names)
	}
	if keywords := filter.GetContentKeyword(); len(keywords) > 0 {
		db = bizhelper.FuzzQueryArrayStringOrLike(db, "content", keywords)
	}
	if typ := filter.GetType(); typ != "" {
		db = db.Where("type = ?", typ)
	}
	return db
}

func CreateHotPatchTemplate(db *gorm.DB, name, content, typ string) error {
	t := NewHotPatchTemplate(name, content, typ)
	return db.Create(&t).Error
}

func DeleteAllHotPatchTemplate(db *gorm.DB) error {
	if ndb := db.DropTableIfExists(&schema.HotPatchTemplate{}); ndb.Error != nil {
		return ndb.Error
	}
	if ndb := db.Exec(fmt.Sprintf(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='%s';`, schema.HotPatchTemplateTableName)); ndb.Error != nil {
		return ndb.Error
	}
	if ndb := db.AutoMigrate(&schema.HotPatchTemplate{}); ndb.Error != nil {
		return ndb.Error
	}
	return nil
}

func DeleteHotPatchTemplate(db *gorm.DB, filter *ypb.HotPatchTemplateRequest) (int64, error) {
	if FilterHotPatchTemplateIsEmpty(filter) {
		return 0, utils.Error(`empty filter`)
	}
	db = db.Model(&schema.HotPatchTemplate{})
	db = FilterHotPatchTemplate(db, filter)

	db = db.Unscoped().Delete(&schema.HotPatchTemplate{})
	return db.RowsAffected, db.Error
}

func UpdateHotPatchTemplate(db *gorm.DB, name, content, typ string, filter *ypb.HotPatchTemplateRequest) (int64, error) {
	db = db.Model(&schema.HotPatchTemplate{})
	db = FilterHotPatchTemplate(db, filter)

	m := make(map[string]any)
	if name != "" {
		m["name"] = name
	}
	if content != "" {
		m["content"] = content
	}
	if typ != "" {
		m["type"] = typ
	}
	db = db.Updates(m)
	return db.RowsAffected, db.Error
}

func UpdateHotPatchTemplateForce(db *gorm.DB, name, content, typ string, filter *ypb.HotPatchTemplateRequest) (int64, error) {
	db = db.Model(&schema.HotPatchTemplate{})
	db = FilterHotPatchTemplate(db, filter)

	db = db.Updates(map[string]any{"name": name, "content": content, "type": typ})
	return db.RowsAffected, db.Error
}

func QueryHotPatchTemplate(db *gorm.DB, filter *ypb.HotPatchTemplateRequest) ([]*schema.HotPatchTemplate, error) {
	db = db.Model(&schema.HotPatchTemplate{})
	db = FilterHotPatchTemplate(db, filter)

	var templates []*schema.HotPatchTemplate
	if err := db.Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

func QueryHotPatchTemplateList(db *gorm.DB, filter *ypb.HotPatchTemplateRequest, p *ypb.Paging) (*bizhelper.Paginator, []string, error) {
	var templates []*schema.HotPatchTemplate

	db = db.Model(&schema.HotPatchTemplate{}).Select("name")
	db = FilterHotPatchTemplate(db, filter)
	db = bizhelper.OrderByPaging(db, p)
	paging, db := bizhelper.PagingByPagination(db, p, &templates)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	names := lo.Map(templates, func(t *schema.HotPatchTemplate, _ int) string {
		return t.Name
	})

	return paging, names, nil
}

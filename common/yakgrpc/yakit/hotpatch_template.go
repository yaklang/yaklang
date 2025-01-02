package yakit

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type QueryHotPatchTemplateConfig struct {
	IDs             []int64
	Names           []string
	ContentKeyWords []string
	Type            string
}

func NewQueryHotPatchTemplateConfig() *QueryHotPatchTemplateConfig {
	return new(QueryHotPatchTemplateConfig)
}

type HotPatchTemplateOption func(*QueryHotPatchTemplateConfig)

func WithHotPatchTemplateIDs(ids ...int64) HotPatchTemplateOption {
	return func(c *QueryHotPatchTemplateConfig) {
		if len(ids) == 0 {
			return
		}
		c.IDs = ids
	}
}

func WithHotPatchTemplateNames(names ...string) HotPatchTemplateOption {
	return func(c *QueryHotPatchTemplateConfig) {
		if len(names) == 0 {
			return
		}
		c.Names = names
	}
}

func WithHotPatchTemplateContentKeyWords(contentKeyWords ...string) HotPatchTemplateOption {
	return func(c *QueryHotPatchTemplateConfig) {
		if len(contentKeyWords) == 0 {
			return
		}
		c.ContentKeyWords = contentKeyWords
	}
}

func WithHotPatchTemplateType(typ string) HotPatchTemplateOption {
	return func(c *QueryHotPatchTemplateConfig) {
		c.Type = typ
	}
}

func (cfg *QueryHotPatchTemplateConfig) ToDBQuery(db *gorm.DB) *gorm.DB {
	db = db.Model(&schema.HotPatchTemplate{})
	if len(cfg.IDs) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", cfg.IDs)
	}
	if len(cfg.Names) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "name", cfg.Names)
	}

	if len(cfg.ContentKeyWords) > 0 {
		db = bizhelper.FuzzQueryArrayStringOrLike(db, "content", cfg.ContentKeyWords)
	}
	if cfg.Type != "" {
		db = db.Where("type = ?", cfg.Type)
	}
	return db
}

func NewHotPatchTemplate(name, content, typ string) *schema.HotPatchTemplate {
	return &schema.HotPatchTemplate{
		Name:    name,
		Content: content,
		Type:    typ,
	}
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

func DeleteHotPatchTemplate(db *gorm.DB, opts ...HotPatchTemplateOption) (int64, error) {
	cfg := NewQueryHotPatchTemplateConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	db = cfg.ToDBQuery(db)

	db = db.Unscoped().Delete(&schema.HotPatchTemplate{})
	return db.RowsAffected, db.Error
}

func UpdateHotPatchTemplate(db *gorm.DB, name, content, typ string, opts ...HotPatchTemplateOption) (int64, error) {
	cfg := NewQueryHotPatchTemplateConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	db = cfg.ToDBQuery(db)
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

func UpdateHotPatchTemplateForce(db *gorm.DB, name, content, typ string, opts ...HotPatchTemplateOption) (int64, error) {
	cfg := NewQueryHotPatchTemplateConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	db = cfg.ToDBQuery(db)

	db = db.Updates(map[string]any{"name": name, "content": content, "type": typ})
	return db.RowsAffected, db.Error
}

func QueryHotPatchTemplate(db *gorm.DB, opts ...HotPatchTemplateOption) ([]*schema.HotPatchTemplate, error) {
	cfg := NewQueryHotPatchTemplateConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	var templates []*schema.HotPatchTemplate
	db = cfg.ToDBQuery(db)
	if err := db.Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

package yakit

import (
	"context"
	"sort"

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
	Tags            []string
}

func normalizeHotPatchTemplateTags(tags []string) []string {
	return utils.RemoveRepeatStringSlice(utils.StringArrayFilterEmpty(tags))
}

func NewHotPatchTemplate(name, content, typ string, tags []string) *schema.HotPatchTemplate {
	return &schema.HotPatchTemplate{
		Name:    name,
		Content: content,
		Type:    typ,
		Tags:    schema.StringSlice(normalizeHotPatchTemplateTags(tags)),
	}
}

func FilterHotPatchTemplateIsEmpty(filter *ypb.HotPatchTemplateRequest) bool {
	if filter == nil {
		return true
	}
	if len(filter.GetId()) == 0 && len(filter.GetName()) == 0 && len(filter.GetContentKeyword()) == 0 && filter.GetType() == "" && len(normalizeHotPatchTemplateTags(filter.GetTags())) == 0 {
		return true
	}
	return false
}

func FilterHotPatchTemplate(db *gorm.DB, filter *ypb.HotPatchTemplateRequest) *gorm.DB {
	if filter == nil {
		return db
	}
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
	if tags := normalizeHotPatchTemplateTags(filter.GetTags()); len(tags) > 0 {
		db = bizhelper.FuzzQueryArrayStringOrLike(db, "tags", tags)
	}
	return db
}

func CreateHotPatchTemplate(db *gorm.DB, name, content, typ string, tags []string) error {
	t := NewHotPatchTemplate(name, content, typ, tags)
	return db.Create(&t).Error
}

func DeleteAllHotPatchTemplate(db *gorm.DB) error {
	return schema.DropRecreateTable(db, &schema.HotPatchTemplate{})
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

func UpdateHotPatchTemplate(db *gorm.DB, name, content, typ string, tags []string, filter *ypb.HotPatchTemplateRequest) (int64, error) {
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
	if normalizedTags := normalizeHotPatchTemplateTags(tags); len(normalizedTags) > 0 {
		m["tags"] = schema.StringSlice(normalizedTags)
	}
	db = db.Updates(m)
	return db.RowsAffected, db.Error
}

func UpdateHotPatchTemplateForce(db *gorm.DB, name, content, typ string, tags []string, filter *ypb.HotPatchTemplateRequest) (int64, error) {
	db = db.Model(&schema.HotPatchTemplate{})
	db = FilterHotPatchTemplate(db, filter)

	db = db.Updates(map[string]any{
		"name":    name,
		"content": content,
		"type":    typ,
		"tags":    schema.StringSlice(normalizeHotPatchTemplateTags(tags)),
	})
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

func CountHotPatchTemplateTags(ctx context.Context, db *gorm.DB) ([]*ypb.Tags, error) {
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	tagCount := make(map[string]int)
	db = db.Model(&schema.HotPatchTemplate{})
	for template := range bizhelper.YieldModel[*schema.HotPatchTemplate](ctx, db) {
		for _, tag := range normalizeHotPatchTemplateTags(template.Tags) {
			tagCount[tag]++
		}
	}

	tags := make([]*ypb.Tags, 0, len(tagCount))
	for tag, count := range tagCount {
		tags = append(tags, &ypb.Tags{
			Value: tag,
			Total: int32(count),
		})
	}

	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Total == tags[j].Total {
			return tags[i].Value < tags[j].Value
		}
		return tags[i].Total > tags[j].Total
	})
	return tags, nil
}

func GetHotPatchTemplate(db *gorm.DB, req *ypb.UploadHotPatchTemplateToOnlineRequest) (*schema.HotPatchTemplate, error) {
	db = db.Model(&schema.HotPatchTemplate{})
	db = db.Where("name = ?", req.GetName()).Where("type = ?", req.GetType())
	var templates schema.HotPatchTemplate
	if err := db.First(&templates).Error; err != nil {
		return nil, err
	}
	return &templates, nil
}

func CreateOrUpdateHotPatchTemplate(db *gorm.DB, name, templateType, content string, tags []string) error {
	condition := NewHotPatchTemplate(name, content, templateType, tags)
	db = db.Model(&schema.HotPatchTemplate{})
	if db = db.Where("name = ? AND type = ?", name, templateType).Assign(condition).FirstOrCreate(&schema.HotPatchTemplate{}); db.Error != nil {
		return utils.Errorf("create or update HotPatchTemplate failed: %s", db.Error)
	}
	return nil
}

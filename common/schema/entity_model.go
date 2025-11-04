package schema

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type EntityRepository struct {
	gorm.Model

	Uuid           string `gorm:"unique_index"`
	EntityBaseName string `gorm:"index"`
	Description    string
}

func (e *EntityRepository) BeforeSave() error {
	if e.Uuid == "" {
		e.Uuid = uuid.NewString()
	}
	return nil
}

func (e *EntityRepository) TableName() string {
	return "rag_entity_repository_test"
}

func (e *EntityRepository) ToGRPC() *ypb.EntityRepository {
	return &ypb.EntityRepository{
		ID:          int64(e.ID),
		Name:        e.EntityBaseName,
		Description: e.Description,
		HiddenIndex: e.Uuid,
	}
}

// ERModelEntity 是知识库中所有事物的基本单元
type ERModelEntity struct {
	gorm.Model

	RepositoryUUID    string      `gorm:"index"`
	EntityName        string      `gorm:"index"`
	Uuid              string      `gorm:"unique_index"`
	Description       string      // 对该实体的简要描述
	EntityType        string      // 实体的类型或类别
	EntityTypeVerbose string      // 实体类型的详细描述
	Attributes        MetadataMap `gorm:"type:text" json:"attributes"`

	RuntimeID string
}

const ERModelEntityBroadcastType = "er_model_entity"

func (a *ERModelEntity) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call(ERModelEntityBroadcastType, "create")
	return nil
}

func (a *ERModelEntity) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call(ERModelEntityBroadcastType, "update")
	return nil
}

func (a *ERModelEntity) AfterDelete(tx *gorm.DB) (err error) {
	broadcastData.Call(ERModelEntityBroadcastType, "delete")
	return nil
}

func (e *ERModelEntity) TableName() string {
	return "rag_entity_test"
}

func (e *ERModelEntity) BeforeSave() error {
	if e.Uuid == "" {
		e.Uuid = uuid.NewString()
	}
	return nil
}

func (e *ERModelEntity) ToGRPC() *ypb.Entity {
	return &ypb.Entity{
		ID:          uint64(e.ID),
		BaseIndex:   e.RepositoryUUID,
		Name:        e.EntityName,
		Type:        e.EntityType,
		Description: e.Description,
		HiddenIndex: e.Uuid,
		Attributes: lo.MapToSlice(e.Attributes, func(key string, value any) *ypb.KVPair {
			return &ypb.KVPair{
				Key:   key,
				Value: utils.InterfaceToString(value),
			}
		}),
	}
}

func EntityGRPCToModel(e *ypb.Entity) *ERModelEntity {
	attributes := MetadataMap{}
	for _, attr := range e.Attributes {
		attributes[attr.GetKey()] = attr.GetValue()
	}

	return &ERModelEntity{
		Model: gorm.Model{
			ID: uint(e.GetID()),
		},
		Uuid:           e.GetHiddenIndex(),
		EntityName:     e.GetName(),
		EntityType:     e.GetType(),
		Description:    e.GetDescription(),
		Attributes:     attributes,
		RepositoryUUID: e.GetBaseIndex(),
	}
}

func (e *ERModelEntity) String() string {
	attrString := strings.Builder{}
	for name, attr := range e.Attributes {
		attrString.WriteString(fmt.Sprintf("%s=%v;", name, attr))
	}
	return fmt.Sprintf("%s|%s|%s|%s", e.EntityName, e.EntityType, e.Description, attrString.String())
}

func (e *ERModelEntity) ToRAGContent() string {
	if e == nil {
		return ""
	}
	attrString := strings.Builder{}
	for name, attr := range e.Attributes {
		attrString.WriteString(fmt.Sprintf("%s=%v;", name, attr))
	}

	result, err := utils.RenderTemplate("{{ .name }}[{{ .type }}{{ if .type_verbose }}({{ .type_verbose }}){{ end }}]"+
		"{{ if .desc }} DESC: {{ .desc }}{{ end }}"+
		"{{ if .attr }} ATTR:{{ .attr }}{{ end }}", map[string]any{
		"name":         e.EntityName,
		"type":         e.EntityType,
		"type_verbose": e.EntityTypeVerbose,
		"desc":         e.Description,
		"attr":         attrString.String(),
	})
	if err != nil {
		return strings.Trim(e.String(), "|")
	}
	return result
}

func (e *ERModelEntity) Dump() string {
	if e == nil {
		return "<nil ERModelEntity>"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- ERModelEntity ---\n"))
	sb.WriteString(fmt.Sprintf("  EntityName:   %s\n", e.EntityName))
	sb.WriteString(fmt.Sprintf("  EntityType:   %s\n", e.EntityType))
	sb.WriteString(fmt.Sprintf("  Description:  %s\n", e.Description))
	sb.WriteString("\n") // 添加空行以分隔
	sb.WriteString(fmt.Sprintf("  Attributes (%d):\n", len(e.Attributes)))
	if len(e.Attributes) == 0 {
		sb.WriteString("    (No attributes)\n")
	} else {
		for name, attr := range e.Attributes {
			sb.WriteString(fmt.Sprintf("    AttributeName:    %s\n", name))
			sb.WriteString(fmt.Sprintf("    sAttributeValue:    %s\n", attr))
		}
	}
	sb.WriteString("--------------------------------\n")
	return sb.String()
}

type ERModelRelationship struct {
	gorm.Model

	Uuid           string `gorm:"unique_index"`
	RepositoryUUID string `gorm:"index"`

	SourceEntityID uint
	TargetEntityID uint

	SourceEntityIndex       string
	TargetEntityIndex       string
	RelationshipType        string
	RelationshipTypeVerbose string
	Hash                    string      `gorm:"unique_index"`
	Attributes              MetadataMap `gorm:"type:text" json:"attributes"`

	RuntimeID string
}

func RelationshipGRPCToModel(r *ypb.Relationship) *ERModelRelationship {
	attributes := MetadataMap{}
	for _, attr := range r.Attributes {
		attributes[attr.GetKey()] = attr.GetValue()
	}

	return &ERModelRelationship{
		Model: gorm.Model{
			ID: uint(r.GetID()),
		},
		Uuid:              r.GetUUID(),
		SourceEntityIndex: r.GetSourceEntityIndex(),
		TargetEntityIndex: r.GetTargetEntityIndex(),
		RelationshipType:  r.GetType(),
		Attributes:        attributes,
		RepositoryUUID:    r.GetRepositoryUUID(),
	}
}

const ERModelRelationshipBroadcastType = "er_model_relationship"

func (r *ERModelRelationship) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call(ERModelRelationshipBroadcastType, "create")
	return nil
}

func (r *ERModelRelationship) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call(ERModelRelationshipBroadcastType, "update")
	return nil
}

func (r *ERModelRelationship) AfterDelete(tx *gorm.DB) (err error) {
	broadcastData.Call(ERModelRelationshipBroadcastType, "delete")
	return nil
}

func (r *ERModelRelationship) TableName() string {
	return "rag_entity_relationship_test"
}

func (r *ERModelRelationship) ToRAGContent(src string, dst string) string {
	var attr bytes.Buffer
	for k, v := range r.Attributes {
		attr.WriteString(fmt.Sprintf("%s=%v; ", k, v))
	}

	result, err := utils.RenderTemplate(`
<|src|> {{ .src }} <|src|> --> {{ .relationship_type }}{{ if .type_verbose }}({{ .type_verbose }}){{ end }} --> <|dst|> {{ .dst }} <|dst|>
{{ if .attr }}attrs: {{ .attr }}{{ end }}
`, map[string]any{
		"relationship_type": r.RelationshipType,
		"type_verbose":      r.RelationshipTypeVerbose,
		"src":               src,
		"dst":               dst,
		"attr":              attr.String(),
	})
	if err != nil {
		log.Error("cannot build relationship RAG content:", err)
		return string(utils.Jsonify(r))
	}
	return result
}

func (r *ERModelRelationship) String() string {
	return fmt.Sprintf("%s [%s:%v] %s", r.SourceEntityIndex, r.RelationshipType, r.RelationshipTypeVerbose, r.TargetEntityIndex)
}

func (r *ERModelRelationship) ToGRPC() *ypb.Relationship {
	return &ypb.Relationship{
		RepositoryUUID:    r.RepositoryUUID,
		UUID:              r.Uuid,
		ID:                uint64(r.ID),
		Type:              r.RelationshipType,
		SourceEntityIndex: r.SourceEntityIndex,
		TargetEntityIndex: r.TargetEntityIndex,
		Attributes: lo.MapToSlice(r.Attributes, func(key string, value any) *ypb.KVPair {
			return &ypb.KVPair{
				Key:   key,
				Value: utils.InterfaceToString(value),
			}
		}),
	}
}

func (r *ERModelRelationship) CalcHash() string {
	return utils.CalcSha1(
		r.RepositoryUUID,
		r.RelationshipType,
		r.Attributes,
		r.SourceEntityIndex,
		r.TargetEntityIndex,
	)
}

func (r *ERModelRelationship) BeforeSave() error {
	r.Hash = r.CalcHash()
	if r.Uuid == "" {
		r.Uuid = uuid.NewString()
	}
	return nil
}

func init() {
	// 注册数据库表结构到系统中
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE,
		&EntityRepository{},
		&ERModelEntity{},
		//&ERModelAttribute{},
		&ERModelRelationship{},
	)

}

func SimpleBuildEntityFilter(reposName string, entityTypes, names, HiddenIndex, keywords []string) *ypb.EntityFilter {
	filter := &ypb.EntityFilter{
		ReposName:   reposName,
		Types:       entityTypes,
		Names:       names,
		HiddenIndex: HiddenIndex,
		Keywords:    keywords,
	}
	return filter
}

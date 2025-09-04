package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func setupTestData(t *testing.T) (entityBaseIndex, entityBaseName string, entityIndex, entityNames []string, relationshipIDs []uint, relationshipType string, clearFunc func()) {
	db := consts.GetGormProfileDatabase()
	// 创建 EntityBase
	entityBaseName = fmt.Sprintf("TestBase_%s", uuid.New().String())
	entityBase := &schema.EntityBaseInfo{
		EntityBaseName: entityBaseName,
		Description:    "Test Description",
		HiddenIndex:    uuid.NewString(),
	}
	if err := db.Create(entityBase).Error; err != nil {
		t.Fatalf("setupTestData: create EntityBaseInfo failed: %v", err)
	}
	entityBaseID := entityBase.ID
	entityBaseIndex = entityBase.HiddenIndex
	// 创建实体
	var entities []*schema.ERModelEntity
	var entityIDs []uint
	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("Entity_%d_%s", i, uuid.New().String())
		e := &schema.ERModelEntity{
			EntityBaseID:    entityBaseID,
			EntityBaseIndex: entityBaseIndex,
			EntityName:      name,
			EntityType:      "TypeA",
			Description:     "Test Entity",
			Rationale:       "Test Rationale",
			Attributes:      schema.MetadataMap{"attr": fmt.Sprintf("val_%d", i)},
		}
		if err := db.Create(e).Error; err != nil {
			t.Fatalf("setupTestData: create ERModelEntity failed: %v", err)
		}
		entities = append(entities, e)
		entityIndex = append(entityIndex, e.HiddenIndex)
		entityIDs = append(entityIDs, e.ID)
		entityNames = append(entityNames, name)
	}

	// 创建关系
	relationshipType = "relType"
	r := &schema.ERModelRelationship{
		EntityBaseID:      entityBaseID,
		SourceEntityID:    entities[0].ID,
		RelationshipType:  relationshipType,
		TargetEntityID:    entities[1].ID,
		DecisionRationale: "Test Relationship",
		Attributes:        schema.MetadataMap{"relAttr": "relVal"},
	}
	r.Hash = r.CalcHash()
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("setupTestData: create ERModelRelationship failed: %v", err)
	}
	relationshipIDs = append(relationshipIDs, r.ID)

	clearFunc = func() {
		for _, rid := range relationshipIDs {
			db.Unscoped().Delete(&schema.ERModelRelationship{}, rid)
		}
		for _, eid := range entityIDs {
			db.Unscoped().Delete(&schema.ERModelEntity{}, eid)
		}
		db.Unscoped().Delete(&schema.EntityBaseInfo{}, entityBaseID)
	}

	return entityBaseIndex, entityBaseName, entityIndex, entityNames, relationshipIDs, relationshipType, clearFunc
}

func TestListEntityRepository(t *testing.T) {
	entityBaseIndex, entityBaseName, _, _, _, _, clearFunc := setupTestData(t)
	t.Cleanup(clearFunc)

	client, err := NewLocalClient()
	if err != nil {
		t.Fatalf("Failed to create local gRPC client: %v", err)
	}
	resp, err := client.ListEntityRepository(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatalf("ListEntityRepository failed: %v", err)
	}
	if resp == nil || len(resp.EntityRepositories) == 0 {
		t.Fatalf("ListEntityRepository returned empty list, expected at least one")
	}
	// 严格检查：是否包含刚插入的 entityBaseName
	found := false
	for _, repo := range resp.EntityRepositories {
		if repo.Name == entityBaseName && repo.HiddenIndex == entityBaseIndex {
			found = true
			if repo.Description != "Test Description" {
				t.Errorf("EntityRepository description mismatch: got %s", repo.Description)
			}
			break
		}
	}
	if !found {
		t.Fatalf("ListEntityRepository: inserted entity base not found")
	}
}

func TestQueryEntity(t *testing.T) {
	entityBaseIndex, _, entityIndex, entityNames, _, _, clearFunc := setupTestData(t)
	t.Cleanup(clearFunc)

	client, err := NewLocalClient()
	if err != nil {
		t.Fatalf("Failed to create local gRPC client: %v", err)
	}
	req := &ypb.QueryEntityRequest{
		Filter:     &ypb.EntityFilter{BaseIndex: entityBaseIndex},
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id"},
	}
	resp, err := client.QueryEntity(context.Background(), req)
	if err != nil {
		t.Fatalf("QueryEntity failed: %v", err)
	}
	if len(resp.Entities) != 2 {
		t.Fatalf("QueryEntity returned %d entities, expected 2", len(resp.Entities))
	}
	// 检查内容
	for i, e := range resp.Entities {
		if e.BaseIndex != entityBaseIndex {
			t.Errorf("Entity Base mismatch: got %s, want %s", e.BaseIndex, entityBaseIndex)
		}
		if e.Type != "TypeA" {
			t.Errorf("Entity Type mismatch: got %s, want TypeA", e.Type)
		}
		if e.Name != entityNames[i] {
			t.Errorf("Entity Name mismatch: got %s, want %s", e.Name, entityNames[i])
		}
		if e.HiddenIndex != entityIndex[i] {
			t.Errorf("Entity BaseIndex mismatch: got %s, want %s", e.HiddenIndex, entityIndex[i])
		}
		if len(e.Attributes) != 1 || e.Attributes[0].Key != "attr" {
			t.Errorf("Entity Attributes mismatch: got %+v", e.Attributes)
		}
	}
	// 检查分页
	if resp.Pagination.Page != 1 || resp.Pagination.Limit != 10 {
		t.Errorf("Pagination mismatch: %+v", resp.Pagination)
	}
}

func TestQueryRelationship(t *testing.T) {
	entityBaseIndex, _, entityIndex, _, _, relationshipType, clearFunc := setupTestData(t)
	t.Cleanup(clearFunc)

	client, err := NewLocalClient()
	if err != nil {

		t.Fatalf("Failed to create local gRPC client: %v", err)
	}
	req := &ypb.QueryRelationshipRequest{
		Filter:     &ypb.RelationshipFilter{BaseIndex: entityBaseIndex},
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id"},
	}
	resp, err := client.QueryRelationship(context.Background(), req)
	if err != nil {
		t.Fatalf("QueryRelationship failed: %v", err)
	}
	if len(resp.Relationships) != 1 {
		t.Fatalf("QueryRelationship returned %d relationships, expected 1", len(resp.Relationships))
	}
	rel := resp.Relationships[0]
	if rel.Type != relationshipType {
		t.Errorf("Relationship Type mismatch: got %s, want %s", rel.Type, relationshipType)
	}

	if rel.SourceEntityIndex != entityIndex[0] || rel.TargetEntityIndex != entityIndex[1] {
		t.Errorf("Relationship EntityIndex mismatch: got %s -> %s, want %s -> %s", rel.SourceEntityIndex, rel.TargetEntityIndex, entityIndex[0], entityIndex[1])
	}

	if len(rel.Attributes) != 1 || rel.Attributes[0].Key != "relAttr" {
		t.Errorf("Relationship Attributes mismatch: got %+v", rel.Attributes)
	}
}

func TestGenerateERMDot(t *testing.T) {
	entityBaseIndex, _, _, entityNames, _, relationshipType, clearFunc := setupTestData(t)
	t.Cleanup(clearFunc)

	client, err := NewLocalClient()
	if err != nil {
		t.Fatalf("Failed to create local gRPC client: %v", err)
	}
	req := &ypb.GenerateERMDotRequest{
		Filter: &ypb.EntityFilter{BaseIndex: entityBaseIndex},
		Depth:  2,
	}
	resp, err := client.GenerateERMDot(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateERMDot failed: %v", err)
	}
	if resp.Dot == "" {
		t.Fatalf("GenerateERMDot returned empty dot string, expected non-empty")
	}
	// 检查 DOT 内容包含实体和关系
	for _, name := range entityNames {
		if !strings.Contains(resp.Dot, name) {
			t.Errorf("DOT output missing entity name: %s", name)
		}
	}
	if !strings.Contains(resp.Dot, relationshipType) {
		t.Errorf("DOT output missing relationship type: %s", relationshipType)
	}
}

package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	BlueprintRelationParents   BlueprintRelationKind = "__parents__"
	BlueprintRelationSuper                           = "__super__"
	BlueprintRelationInterface                       = "__interface__"

	BlueprintRelationChildren = "__children__"
	BlueprintRelationSub      = "__sub__"
	BlueprintRelationImpl     = "__impl__"
)

func (b BlueprintRelationKind) getRelativeRelation() BlueprintRelationKind {
	switch b {
	case BlueprintRelationParents:
		return BlueprintRelationChildren
	case BlueprintRelationSuper:
		return BlueprintRelationSub
	case BlueprintRelationInterface:
		return BlueprintRelationImpl
	case BlueprintRelationChildren:
		return BlueprintRelationParents
	case BlueprintRelationSub:
		return BlueprintRelationSuper
	case BlueprintRelationImpl:
		return BlueprintRelationInterface
	}
	return ""
}

func (c *Blueprint) setBlueprintRelation(parent *Blueprint, relation BlueprintRelationKind) {
	if parent == nil || c == nil {
		return
	}
	switch relation {
	case BlueprintRelationParents:
		c.ParentBlueprints = append(c.ParentBlueprints, parent)
	case BlueprintRelationSuper:
		c.SuperBlueprints = append(c.SuperBlueprints, parent)
	case BlueprintRelationInterface:
		c.InterfaceBlueprints = append(c.InterfaceBlueprints, parent)
	default:
		log.Errorf("BUG!: add parent blueprint error: unknown relation %v", relation)
		return
	}
	c.storeBlueprintRelation(parent, relation)
}

func (c *Blueprint) storeBlueprintRelation(other *Blueprint, relation BlueprintRelationKind) {
	if utils.IsNil(c) || utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	if utils.IsNil(other) || utils.IsNil(other._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}

	builder := c._container.GetFunc().builder
	val := builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(string(relation)))
	builder.AssignVariable(val, other._container)
	// set relative relation
	otherBuilder := other._container.GetFunc().builder
	relativeRela := relation.getRelativeRelation()
	if string(relativeRela) == "" {
		return
	}
	otherVal := otherBuilder.CreateMemberCallVariable(other._container, otherBuilder.EmitConstInst(string(relativeRela)))
	otherBuilder.AssignVariable(otherVal, c._container)
}

func (c *Blueprint) IsInterface() bool {
	if c == nil {
		return false
	}
	return c.Kind == BlueprintInterface
}

func (c *Blueprint) IsClass() bool {
	if c == nil {
		return false
	}
	return c.Kind == BlueprintClass
}

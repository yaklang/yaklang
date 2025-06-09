package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	BlueprintRelationParents   BlueprintRelationKind = "__parents__"
	BlueprintRelationInterface                       = "__interface__"

	BlueprintRelationInherit = "__inherit__"
	BlueprintRelationImpl    = "__implement__"
)

func (b BlueprintRelationKind) getRelativeRelation() BlueprintRelationKind {
	switch b {
	case BlueprintRelationParents:
		return BlueprintRelationInherit
	case BlueprintRelationInterface:
		return BlueprintRelationImpl
	case BlueprintRelationImpl:
		return BlueprintRelationInterface
	case BlueprintRelationInherit:
		return BlueprintRelationParents
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

	// assign will fix value's verbose name
	// but for blueprint, we need to keep the original verbose name
	cName := c._container.GetVerboseName()
	otherName := other._container.GetVerboseName()

	builder := c._container.GetFunc().builder
	val := builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(string(relation), true))
	builder.AssignVariable(val, other._container)
	other._container.SetVerboseName(otherName)
	// set relative relation
	otherBuilder := other._container.GetFunc().builder
	relativeRela := relation.getRelativeRelation()
	if string(relativeRela) == "" {
		return
	}
	otherVal := otherBuilder.CreateMemberCallVariable(other._container, otherBuilder.EmitConstInst(string(relativeRela), true))
	otherBuilder.AssignVariable(otherVal, c._container)
	c._container.SetVerboseName(cName)
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

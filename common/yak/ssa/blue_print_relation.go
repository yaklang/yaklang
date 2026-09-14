package ssa

import (
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

func (c *Blueprint) setBlueprintRelation(parent *Blueprint, relation BlueprintRelationKind) bool {
	if parent == nil || c == nil {
		return false
	}
	switch relation {
	case BlueprintRelationParents:
		for _, existed := range c.ParentBlueprints {
			if existed == parent {
				return false
			}
		}
		c.ParentBlueprints = append(c.ParentBlueprints, parent)
	case BlueprintRelationInterface:
		for _, existed := range c.InterfaceBlueprints {
			if existed == parent {
				return false
			}
		}
		c.InterfaceBlueprints = append(c.InterfaceBlueprints, parent)
	default:
		log.Errorf("BUG!: add parent blueprint error: unknown relation %v", relation)
		return false
	}
	c.storeBlueprintRelation(parent, relation)
	return true
}

func (c *Blueprint) storeBlueprintRelation(other *Blueprint, relation BlueprintRelationKind) {
	if utils.IsNil(c) || utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	if utils.IsNil(other) || utils.IsNil(other._container) || utils.IsNil(other._container.GetFunc()) {
		return
	}

	// assign will fix value's verbose name
	// but for blueprint, we need to keep the original verbose name
	cName := c._container.GetVerboseName()
	otherName := other._container.GetVerboseName()

	storeRelation := func(builder *FunctionBuilder, container Value, key BlueprintRelationKind, target Value) {
		relationKey := builder.EmitConstInstPlaceholder(string(key))
		// Blueprint relations are SSA-owned metadata rather than source-language
		// members. Register the known edge before the generic member-variable path
		// validates it, otherwise class-aware frontends report an ObjectError for
		// these internal keys. AssignVariable records the normal variable metadata;
		// the identical member edge is de-duplicated by AddMember.
		setMemberCallRelationship(container, relationKey, target)
		variable := builder.CreateMemberCallVariable(container, relationKey)
		builder.AssignVariable(variable, target)
	}

	builder := c._container.GetFunc().builder
	storeRelation(builder, c._container, relation, other._container)
	other._container.SetVerboseName(otherName)
	// set relative relation
	otherBuilder := other._container.GetFunc().builder
	relativeRela := relation.getRelativeRelation()
	if string(relativeRela) == "" {
		return
	}
	storeRelation(otherBuilder, other._container, relativeRela, c._container)
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

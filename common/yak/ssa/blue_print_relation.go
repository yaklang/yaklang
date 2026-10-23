package ssa

import (
	"fmt"
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

	// These edges describe types, not source-language assignments. In particular,
	// recording the reverse edge must not enter the dependency's builder: doing
	// so mutates its scope and may recursively start its field initializers.
	storeRelation := func(container Value, key BlueprintRelationKind, target Value) {
		relationKey := NewConst(string(key), true)
		relationKey.SetRange(container.GetRange())
		relationKey.SetBlock(container.GetBlock())
		relationKey.SetFunc(container.GetFunc())
		container.GetProgram().SetVirtualRegister(relationKey)
		setMemberCallRelationship(container, relationKey, target)
		// Global member queries (including DB reload) use the member index,
		// which source assignments normally populate through variable creation.
		// Publish that index explicitly without creating a source-scope variable.
		container.GetProgram().Cache.AddVariable(fmt.Sprintf("#%d.%s", container.GetId(), key), target)
	}
	storeRelation(c._container, relation, other._container)
	if relative := relation.getRelativeRelation(); relative != "" {
		storeRelation(other._container, relative, c._container)
	}
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

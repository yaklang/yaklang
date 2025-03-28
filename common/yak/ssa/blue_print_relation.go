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

func (b *Blueprint) setBlueprintRelation(parent *Blueprint, relation BlueprintRelationKind) {
	if parent == nil || b == nil {
		return
	}
	switch relation {
	case BlueprintRelationParents:
		b.ParentBlueprints = append(b.ParentBlueprints, parent)
	case BlueprintRelationSuper:
		b.SuperBlueprints = append(b.SuperBlueprints, parent)
	case BlueprintRelationInterface:
		b.InterfaceBlueprints = append(b.InterfaceBlueprints, parent)
	default:
		log.Errorf("BUG!: add parent blueprint error: unknown relation %v", relation)
		return
	}
	b.storeBlueprintRelation(parent, relation)
}

func (b *Blueprint) storeBlueprintRelation(other *Blueprint, relation BlueprintRelationKind) {
	if utils.IsNil(b) || utils.IsNil(b._container) || utils.IsNil(b._container.GetFunc()) {
		return
	}
	if utils.IsNil(other) || utils.IsNil(other._container) || utils.IsNil(b._container.GetFunc()) {
		return
	}

	// assign will fix value's verbose name
	// but for blueprint, we need to keep the original verbose name
	cName := b._container.GetVerboseName()
	otherName := other._container.GetVerboseName()

	builder := b._container.GetFunc().builder
	val := builder.CreateMemberCallVariable(b._container, builder.EmitConstInst(string(relation)))
	builder.AssignVariable(val, other._container)
	other._container.SetVerboseName(otherName)
	// set relative relation
	otherBuilder := other._container.GetFunc().builder
	relativeRela := relation.getRelativeRelation()
	if string(relativeRela) == "" {
		return
	}
	otherVal := otherBuilder.CreateMemberCallVariable(other._container, otherBuilder.EmitConstInst(string(relativeRela)))
	otherBuilder.AssignVariable(otherVal, b._container)
	b._container.SetVerboseName(cName)
}

func (b *Blueprint) IsInterface() bool {
	if b == nil {
		return false
	}
	return b.Kind == BlueprintInterface
}

func (b *Blueprint) IsClass() bool {
	if b == nil {
		return false
	}
	return b.Kind == BlueprintClass
}

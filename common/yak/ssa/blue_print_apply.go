package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

// ParseClassBluePrint  parse get classBluePrint if the ObjectType is a ClassFactor
func ParseClassBluePrint(this Value, objectTyp *ObjectType) (ret Type) {
	ret = objectTyp

	if !this.IsObject() {
		return
	}
	blue := NewBlueprint(objectTyp.Name)

	for key, member := range this.GetAllMember() {
		// if not function , just append this field to normal field
		typ := member.GetType()
		if typ.GetTypeKind() != FunctionTypeKind {
			// blue.NormalMember[key.String()] = member
			blue.RegisterNormalMember(key.String(), member)
			continue
		}

		funTyp := typ.(*FunctionType)
		if len(funTyp.ParameterValue) > 0 {
			if para := funTyp.ParameterValue[0]; para != nil && (para.IsObject() || para.HasUsers()) {
				blue.AddMethod(key.String(), funTyp.This)
				continue
			}
		}

		blue.RegisterNormalMember(key.String(), member)
	}

	if len(blue.GetMethod()) != 0 {
		return blue
	}

	return
}

func (c *Blueprint) Apply(obj Value) Type {
	if c == nil {
		log.Error("BUG: ClassBluePrint is nil")
		log.Error("BUG: ClassBluePrint is nil")
		log.Error("BUG: ClassBluePrint is nil")
		log.Error("BUG: ClassBluePrint is nil")
		return NewAny().GetType()
	}
	if obj == nil {
		log.Errorf("BUG: ClassBluePrint.Apply: obj is nil, c: %v", c)
		return NewAny().GetType()
	}

	fun := obj.GetFunc()
	if fun == nil {
		log.Errorf("BUG: ClassBluePrint.Apply: obj.GetFunc() is nil, obj: %v", obj)
		return NewAny().GetType()
	}
	builder := fun.builder
	_ = builder
	if builder == nil {
		log.Errorf("BUG: ClassBluePrint.Apply: fun.builder is nil, fun: %v", fun)
		return CreateAnyType()
	}
	prog := builder.GetProgram()

	for _, parent := range c.ParentBlueprints {
		if parent == nil {
			log.Warn("ClassBluePrint.ParentBlueprints is nil")
			log.Warn("ClassBluePrint.ParentBlueprints is nil")
			log.Warn("ClassBluePrint.ParentBlueprints is nil")
			continue
		}
		parent.Apply(obj)
		_, i := lo.Difference(c.fullTypeName, parent.fullTypeName)
		c.fullTypeName = append(c.fullTypeName, i...)
	}

	for _, bp := range c.InterfaceBlueprints {
		if bp == nil {
			log.Warn("ClassBluePrint.ParentBlueprints is nil")
			log.Warn("ClassBluePrint.ParentBlueprints is nil")
			log.Warn("ClassBluePrint.ParentBlueprints is nil")
			continue
		}
		bp.Apply(obj)
		_, i := lo.Difference(c.fullTypeName, bp.fullTypeName)
		c.fullTypeName = append(c.fullTypeName, i...)
	}

	if prog != nil || prog.Cache != nil {
		prog.Cache.AddClassInstance(c.Name, obj)
	}

	// if this builder support class, just return the class
	if builder.isSupportClass() {
		return c
	}

	call, isCall := ToCall(obj)

	objTyp := NewObjectType()
	objTyp.SetName(c.Name)
	objTyp.SetMethod(c.GetMethod())
	for _, parent := range c.ParentBlueprints {
		parent.Apply(obj)
	}

	for rawKey, member := range c.NormalMember {
		typ := member.GetType()
		value := member
		key := builder.EmitConstInstPlaceholder(rawKey)
		log.Infof("apply key: %s, member: %v", key, member)

		objTyp.AddField(key, typ)

		// if in yaklang code, classBluePrint only create by function,
		// and Apply only called by function'call (call instruction)
		// and only this language, member can be set by `Parameter`,
		// we just create side-effect
		if para, ok := ToParameter(value); ok && isCall {
			sideEffect := builder.EmitSideEffect(key.String(), call, para)
			builder.AssignVariable(
				builder.CreateMemberCallVariable(obj, key),
				sideEffect,
			)
			continue
		}

		// in other language supported class,
		// classBluePrint only create by `class` keyword.
		// in this case, member can be set nil, just declare the type.
		if utils.IsNil(value) {
			value := builder.ReadMemberCallValue(obj, key)
			value.SetType(typ)
		} else {
			builder.AssignVariable(
				builder.CreateMemberCallVariable(obj, key),
				value,
			)
		}
	}

	objTyp.fullTypeName = c.GetFullTypeNames()
	return objTyp
}

//go:build !no_language
// +build !no_language

package java2ssa

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *singleFileBuilder) VisitModifiers(raw javaparser.IModifiersContext) (instanceCallback []func(ssa.Value), defCallback []func(ssa.Value), isStatic bool) {
	isStatic = false
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ModifiersContext)
	if i == nil {
		return
	}

	for _, raw := range i.AllModifier() {
		i, ok := raw.(*javaparser.ModifierContext)
		if !ok {
			continue
		}
		_ = i
		if annotation := i.Annotation(); annotation != nil {
			insCallback, defCallbackHandler := y.VisitAnnotation(annotation)
			instanceCallback = append(instanceCallback, insCallback)
			defCallback = append(defCallback, defCallbackHandler)
		} else if modifier := i.StaticClassModifier(); modifier != nil {
			res := y.VisitStaticClassModifier(modifier)
			if res == ssa.Static {
				isStatic = true
			}
		} else if modifier := i.StaticModifier(); modifier != nil {
			res := y.VisitStaticModifier(modifier)
			if res == ssa.Static {
				isStatic = true
			}
		} else {
			log.Errorf("visit modifier error type: %v", i)
		}
	}
	return
}

type AnnotationDescription struct {
	Name string
}

func (y *singleFileBuilder) VisitAnnotation(annotationContext javaparser.IAnnotationContext) (instanceCallback func(ssa.Value), defCallback func(ssa.Value)) {
	start := time.Now()
	defer deltaAnnotationCostFrom(start)
	recoverRange := y.SetRange(annotationContext)
	defer recoverRange()

	instanceCallback = func(ssa.Value) {}
	defCallback = func(ssa.Value) {}

	if y == nil || annotationContext == nil {
		return
	}
	i, _ := annotationContext.(*javaparser.AnnotationContext)
	if i == nil {
		return
	}

	var annotationName string
	var annotationRange = y.GetRangeByToken(annotationContext)

	if ret := i.AltAnnotationQualifiedName(); ret != nil {
		annotationName = ret.GetText()
		if !strings.HasPrefix(annotationName, "@") {
			log.Warnf("bad syntax... why altAnnotation name %#v is not prefix with @? use str after @", annotationName)
			_, annotationName, _ = strings.Cut(annotationName, "@")
		} else {
			annotationName = strings.TrimLeft(annotationName, "@")
		}
	} else {
		annotationName = i.QualifiedName().GetText()
	}

	data := make(map[string]ssa.Value)
	if ret := i.ElementValue(); ret != nil {
		data["value"] = y.VisitElementValue(ret)
	} else if ret, _ := i.ElementValuePairs().(*javaparser.ElementValuePairsContext); ret != nil {
		for _, elementPair := range ret.AllElementValuePair() {
			name, v := y.VisitElementValuePair(elementPair)
			data[name] = v
		}
	}

	var annotationContainerVariable *ssa.Variable
	var annotationContainerInstance ssa.Value
	if annotationName != "" {
		// support for spring boot
		if annotationName == "Controller" || annotationName == "RestController" {
			y.isInController = true
		}

		val := y.CreateVariable(annotationName, annotationContext)
		container := y.EmitEmptyContainer()
		log.Infof("create annotation container[%v]: %v", container.GetId(), annotationName)
		y.AssignVariable(val, container)
		annotationContainerVariable = val
		annotationContainerInstance = container
		for name, member := range data {
			val := y.CreateMemberCallVariable(container, y.EmitConstInstPlaceholder(name))
			val.AddRange(annotationRange, true)
			log.Infof("create annotation-key: %v.%v -> %v", annotationName, name, member)
			y.AssignVariable(val, member)
		}
	}

	// 这样要存一份functionBuilder的状态
	// 因为注解回调函数会被setType使用
	// 而不仅是在本functionBuilder
	store := y.StoreFunctionBuilder()
	return func(v ssa.Value) {
			start := time.Now()
			defer deltaAnnotationCostFrom(start)

			switchHandler := y.SwitchFunctionBuilder(store)
			defer switchHandler()
			recoverRange := y.SetCurrent(v)
			defer recoverRange()

			annotation := y.ReadMemberCallValue(v, y.EmitConstInstPlaceholder("annotation"))
			thisAnnotation := y.ReadMemberCallValue(annotation, y.EmitConstInstPlaceholder(annotationName))
			for name, v := range data {
				variable := y.CreateMemberCallVariable(thisAnnotation, y.EmitConstInstPlaceholder(name))
				y.AssignVariable(variable, v)
			}
		}, func(value ssa.Value) {
			/*
				@RequestParam(value = "xml_str") String xmlStr

				means
					xmlStr.annotation.RequestParam.value = "xml_str"
					RequestParam.__ref__ = xmlStr
			*/
			switchHandler := y.SwitchFunctionBuilder(store)
			defer switchHandler()
			start := time.Now()
			defer deltaAnnotationCostFrom(start)

			// function instance
			// parameter instance
			if annotationContainerVariable == nil || annotationContainerInstance == nil {
				return
			}
			// create @RequestMap.ref -> @RequestMap (ref or _)
			log.Infof("start to build annotation ref to def: (%v)%v", value.GetId(), value.GetName())
			annotationToRef := "__ref__"
			ref := y.CreateMemberCallVariable(annotationContainerInstance, y.EmitConstInstPlaceholder(annotationToRef))
			y.AssignVariable(ref, value)
			//for _, v := range annotationContainerInstance.GetAllMember() {
			//	y.AssignVariable(y.CreateMemberCallVariable(v, y.EmitConstInst(annotationToRef)), value)
			//}
			annotationContainer := y.CreateMemberCallVariable(value, y.EmitConstInstPlaceholder("annotation"))
			annotationCollector := y.EmitEmptyContainer()

			y.AssignVariable(annotationContainer, annotationCollector)
			var fieldAnnotationName = annotationName
			if annotationName == "" {
				fieldAnnotationName = annotationContainerInstance.GetName()
			}
			y.AssignVariable(y.CreateMemberCallVariable(annotationCollector, y.EmitConstInstPlaceholder(fieldAnnotationName)), annotationContainerInstance)
			// set fullType Name
			t := y.AddFullTypeNameFromMap(annotationName, annotationContainerInstance.GetType())
			annotationContainerInstance.SetType(t)
		}
}

func (y *singleFileBuilder) VisitStaticModifier(raw javaparser.IStaticModifierContext) ssa.BlueprintModifier {
	return ssa.NoneModifier
}

func (y *singleFileBuilder) VisitStaticClassModifier(raw javaparser.IStaticClassModifierContext) ssa.BlueprintModifier {
	if y == nil || raw == nil || y.IsStop() {
		return ssa.NoneModifier
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.StaticClassModifierContext)
	if i == nil {
		return ssa.NoneModifier
	}

	if i.PUBLIC() != nil {
		return ssa.Public
	} else if i.PROTECTED() != nil {
		return ssa.Protected
	} else if i.PRIVATE() != nil {
		return ssa.Private
	} else if i.STATIC() != nil {
		return ssa.Static
	} else if i.ABSTRACT() != nil {
		return ssa.Abstract
	} else if i.FINAL() != nil {
		return ssa.Final
	} else {
		return ssa.NoneModifier
	}
}

func (y *singleFileBuilder) VisitElementValuePair(raw javaparser.IElementValuePairContext) (name string, v ssa.Value) {
	name = ""
	v = nil
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ElementValuePairContext)
	if i == nil {
		return
	}
	name = i.Identifier().GetText()
	v = y.VisitElementValue(i.ElementValue())

	return
}

func (y *singleFileBuilder) VisitElementValue(raw javaparser.IElementValueContext) (v ssa.Value) {
	v = nil
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ElementValueContext)
	if i == nil {
		return
	}

	if ret := i.Expression(); ret != nil {
		return y.VisitExpression(ret)
	} else if ret := i.Annotation(); ret != nil {
		//TODO: handler element value

	} else if ret := i.ElementValueArrayInitializer(); ret != nil {
		return y.VisitElementValueArrayInitializer(ret)
	} else {
		// log.Errorf("")
	}

	return
}

func (y *singleFileBuilder) VisitElementValueArrayInitializer(raw javaparser.IElementValueArrayInitializerContext) (v ssa.Value) {
	v = nil
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ElementValueArrayInitializerContext)
	if i == nil {
		return
	}

	for _, evContext := range i.AllElementValue() {
		y.VisitElementValue(evContext)
	}

	return nil
}

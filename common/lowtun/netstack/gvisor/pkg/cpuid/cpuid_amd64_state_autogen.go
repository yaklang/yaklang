// automatically generated by stateify.

//go:build amd64 && amd64 && amd64 && amd64 && amd64
// +build amd64,amd64,amd64,amd64,amd64

package cpuid

import (
	"context"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/state"
)

func (fs *FeatureSet) StateTypeName() string {
	return "pkg/cpuid.FeatureSet"
}

func (fs *FeatureSet) StateFields() []string {
	return []string{
		"Function",
		"hwCap",
	}
}

func (fs *FeatureSet) beforeSave() {}

// +checklocksignore
func (fs *FeatureSet) StateSave(stateSinkObject state.Sink) {
	fs.beforeSave()
	var FunctionValue Static
	FunctionValue = fs.saveFunction()
	stateSinkObject.SaveValue(0, FunctionValue)
	stateSinkObject.Save(1, &fs.hwCap)
}

func (fs *FeatureSet) afterLoad(context.Context) {}

// +checklocksignore
func (fs *FeatureSet) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(1, &fs.hwCap)
	stateSourceObject.LoadValue(0, new(Static), func(y any) { fs.loadFunction(ctx, y.(Static)) })
}

func (i *In) StateTypeName() string {
	return "pkg/cpuid.In"
}

func (i *In) StateFields() []string {
	return []string{
		"Eax",
		"Ecx",
	}
}

func (i *In) beforeSave() {}

// +checklocksignore
func (i *In) StateSave(stateSinkObject state.Sink) {
	i.beforeSave()
	stateSinkObject.Save(0, &i.Eax)
	stateSinkObject.Save(1, &i.Ecx)
}

func (i *In) afterLoad(context.Context) {}

// +checklocksignore
func (i *In) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(0, &i.Eax)
	stateSourceObject.Load(1, &i.Ecx)
}

func (o *Out) StateTypeName() string {
	return "pkg/cpuid.Out"
}

func (o *Out) StateFields() []string {
	return []string{
		"Eax",
		"Ebx",
		"Ecx",
		"Edx",
	}
}

func (o *Out) beforeSave() {}

// +checklocksignore
func (o *Out) StateSave(stateSinkObject state.Sink) {
	o.beforeSave()
	stateSinkObject.Save(0, &o.Eax)
	stateSinkObject.Save(1, &o.Ebx)
	stateSinkObject.Save(2, &o.Ecx)
	stateSinkObject.Save(3, &o.Edx)
}

func (o *Out) afterLoad(context.Context) {}

// +checklocksignore
func (o *Out) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(0, &o.Eax)
	stateSourceObject.Load(1, &o.Ebx)
	stateSourceObject.Load(2, &o.Ecx)
	stateSourceObject.Load(3, &o.Edx)
}

func init() {
	state.Register((*FeatureSet)(nil))
	state.Register((*In)(nil))
	state.Register((*Out)(nil))
}
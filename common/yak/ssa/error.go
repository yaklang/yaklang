package ssa

import "fmt"

type ErrorKind int

const (
	Warn ErrorKind = iota
	Error
)

type SSAError struct {
	Pos     *Position
	Message string
	Kind    ErrorKind
}

type SSAErrors []*SSAError

func (f *Function) NewErrorWithPos(kind ErrorKind, Pos *Position, format string, arg ...any) {
	f.err = append(f.err, &SSAError{
		Pos:     Pos,
		Message: fmt.Sprintf(format, arg...),
		Kind:    kind,
	})
}
func (b *FunctionBuilder) NewError(kind ErrorKind, format string, arg ...any) {
	b.NewErrorWithPos(kind, b.currtenPos, format, arg...)
}
func (an anInstruction) NewError(kind ErrorKind, format string, arg ...any) {
	an.Func.NewErrorWithPos(kind, an.pos, format, arg...)
}

func (prog *Program) GetErrors() SSAErrors {
	result := make(SSAErrors, 0)
	for _, pkg := range prog.Packages {
		for _, fun := range pkg.Funcs {
			result = append(result, fun.err...)
		}
	}
	return result
}

func (errs SSAErrors) String() string {
	ret := "error:\n"
	for _, e := range errs {
		ret += "\t" + e.String() + "\n"
	}
	return ret
}

func (err SSAError) String() string {
	ret := ""
	switch err.Kind {
	case Warn:
		ret += "warn:"
	case Error:
		ret += "error:"
	}
	return ret + err.Message
}

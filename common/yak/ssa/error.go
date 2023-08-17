package ssa

import "fmt"

type ErrorKind int

const (
	Info ErrorKind = iota
	Warn
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
func (f *Function) NewError(kind ErrorKind, format string, arg ...any) {
	f.NewErrorWithPos(kind, f.currtenPos, format, arg...)
}
func (an anInstruction) NewError(kind ErrorKind, format string, arg ...any) {
	an.Func.NewErrorWithPos(kind, an.pos, format, arg...)
}

func (prog *Program) GetErrors() SSAErrors {
	result := make(SSAErrors, 0)
	for _, pkg := range prog.Packages {
		for _, fun := range pkg.funcs {
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
	case Info:
		ret += "info:"
	case Warn:
		ret += "warn:"
	case Error:
		ret += "error:"
	}
	return ret + err.Message
}

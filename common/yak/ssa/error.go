package ssa

import "fmt"

type ErrorKind int

const (
	Warn ErrorKind = iota
	Error
)

type ErrorTag string

const (
	SSATAG ErrorTag = "ssa"
)

type SSAError struct {
	Pos     *Position
	tag     ErrorTag
	Message string
	Kind    ErrorKind
}

type SSAErrors []*SSAError

func (f *Function) NewErrorWithPos(kind ErrorKind, tag ErrorTag, Pos *Position, message string) {
	if Pos == nil {
		return
	}
	f.err = append(f.err, &SSAError{
		Pos:     Pos,
		tag:     tag,
		Message: message,
		Kind:    kind,
	})
}
func (b *FunctionBuilder) NewError(kind ErrorKind, tag ErrorTag, format string, arg ...any) {
	b.NewErrorWithPos(kind, tag, b.CurrentPos, format)
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
	var kind string
	switch err.Kind {
	case Warn:
		kind = "warn"
	case Error:
		kind = "error"
	}

	return fmt.Sprintf("[%5s]\t(%s):\t%s", kind, string(err.tag), err.Message)
}

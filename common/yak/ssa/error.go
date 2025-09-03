package ssa

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

type ErrorKind int

const (
	Warn ErrorKind = iota
	Error
)

type ErrorTag string

const (
	SSATAG ErrorTag = "ssa"
)

type ErrorCommentId string
type ErrorComment struct {
	ignorePos []int
	noCheck   bool
}

const (
	SSAIgnore  ErrorCommentId = "// @ssa-ignore"
	SSANoCheck ErrorCommentId = "// @ssa-nocheck"
)

func (ec ErrorComment) Skip(pos *memedit.Range) bool {
	if ec.noCheck {
		return true
	}
	for _, line := range ec.ignorePos {
		if int(pos.GetStart().GetLine()) == line+1 {
			return true
		}
	}
	return false
}

func (f *Function) AddErrorComment(str string, line int) error {
	switch ErrorCommentId(strings.TrimSpace(str)) {
	case SSAIgnore:
		{
			f.errComment.ignorePos = append(f.errComment.ignorePos, line)
		}
	case SSANoCheck:
		if line == 1 {
			f.errComment.noCheck = true
		} else {
			return errors.New(NoCheckMustInFirst())
		}
	default:
		// skip
	}
	return nil
}

type SSAError struct {
	Pos     *memedit.Range
	Tag     ErrorTag
	Message string
	Kind    ErrorKind
}

type SSAErrors []*SSAError

func (f *Function) NewErrorWithPos(kind ErrorKind, tag ErrorTag, Pos *memedit.Range, message string) {
	if Pos == nil {
		return
	}
	if f.errComment.Skip(Pos) {
		return
	}

	prog := f.GetProgram()
	prog.AddError(&SSAError{
		Pos:     Pos,
		Tag:     tag,
		Message: message,
		Kind:    kind,
	})
}
func (b *FunctionBuilder) NewError(kind ErrorKind, tag ErrorTag, massage string, arg ...interface{}) {
	b.NewErrorWithPos(kind, tag, b.CurrentRange, massage)
}

func (f *Function) NewError(kind ErrorKind, tag ErrorTag, format string) {
	f.NewErrorWithPos(kind, tag, f.GetRange(), format)
}

func (prog *Program) AddError(err *SSAError) {
	app := prog.GetApplication()
	app.errors = append(app.errors, err)
}
func (prog *Program) GetErrors() SSAErrors {
	errs := prog.errors
	return errs
}

func (errs SSAErrors) String() string {
	if len(errs) <= 0 {
		return ""
	}
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

	return fmt.Sprintf("[%5s]\t(%s):\t%s: %s", kind, string(err.Tag), err.Message, err.Pos)
}

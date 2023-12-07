package ssa

import (
	"errors"
	"fmt"
	"strings"
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

func (ec ErrorComment) Skip(pos *Position) bool {
	if ec.noCheck {
		return true
	}
	for _, line := range ec.ignorePos {
		if pos.StartLine == line+1 {
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
	if f.errComment.Skip(Pos) {
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

	prog.EachFunction(func(f *Function) {
		result = append(result, f.err...)
	})
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

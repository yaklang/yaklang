package aiknowledge

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
)

type operationError struct {
	code  string
	cause error
}

func (e *operationError) Error() string { return e.cause.Error() }
func (e *operationError) Unwrap() error { return e.cause }

func withErrorCode(code string, err error) error { return &operationError{code: code, cause: err} }
func invalidArgumentf(format string, args ...any) error {
	return withErrorCode("INVALID_ARGUMENT", fmt.Errorf(format, args...))
}

// ErrorResult serializes a business failure for AI tools without raising a
// YakVM panic. A completed tool invocation can still report success=false here.
// Native Go/Yak callers retain the usual result/error return values.
func ErrorResult(err error) map[string]any {
	if err == nil {
		return nil
	}
	code := "OPERATION_FAILED"
	var operation *operationError
	switch {
	case errors.Is(err, context.Canceled):
		code = "CANCELLED"
	case errors.Is(err, context.DeadlineExceeded):
		code = "DEADLINE_EXCEEDED"
	case errors.As(err, &operation):
		code = operation.code
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, fs.ErrNotExist):
		code = "NOT_FOUND"
	case errors.Is(err, knowledgebase.ErrAlreadyExists):
		code = "ALREADY_EXISTS"
	}
	return map[string]any{"success": false, "operation_status": "error", "error_code": code, "message": err.Error()}
}

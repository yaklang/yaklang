package asyncexecmanager

import (
	"context"
	"fmt"
	"reflect"
)

// Resource owns the actual execution and its observations. Inspector must be
// cheap to obtain, stable, and safe to use concurrently with Execute. Execute
// runs once and must honor ctx, join its workers and finish cleanup before return.
// A successful return describes execution completion, not a business verdict.
type Resource interface {
	Execute(ctx context.Context) error
	Inspector() Inspector
}

func resourceInspector(resource Resource) (observer Inspector, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("obtain execution inspector: %v", value)
		}
	}()
	observer = resource.Inspector()
	if isNil(observer) {
		return nil, ErrInvalidResource
	}
	return observer, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

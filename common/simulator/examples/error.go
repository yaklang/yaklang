package examples

import "fmt"

type WrongUsernamePasswordError struct {
	info string
}

func (err *WrongUsernamePasswordError) Error() string {
	return err.info
}

func NewWrongUsernamePasswordError(i interface{}) *WrongUsernamePasswordError {
	return &WrongUsernamePasswordError{
		info: fmt.Sprint(i),
	}
}

func NewWrongUsernamePasswordErrorf(origin string, args ...interface{}) *WrongUsernamePasswordError {
	return &WrongUsernamePasswordError{
		info: fmt.Sprintf(origin, args...),
	}
}

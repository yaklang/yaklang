package utils

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		err  string
		want error
	}{
		{"", fmt.Errorf("")},
		{"foo", fmt.Errorf("foo")},
		{"foo", Error("foo")},
		{"foo bar", Errorf("%s %s", "foo", "bar")},
		{"string with format specifiers: %v", errors.New("string with format specifiers: %v")},
	}

	for _, tt := range tests {
		got := Error(tt.err)
		if got.Error() != tt.want.Error() {
			t.Errorf("New.Error(): got: %q, want %q", got, tt.want)
		}
	}
}

func TestWrapNil(t *testing.T) {
	got := Wrap(nil, "no error")
	if got != nil {
		t.Errorf("Wrap(nil, \"no error\"): got %#v, expected nil", got)
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		err     error
		message string
		want    string
	}{
		{io.EOF, "read error", "read error: EOF"},
		{Wrap(io.EOF, "read error"), "client error", "client error: read error: EOF"},
	}

	for _, tt := range tests {
		got := Wrap(tt.err, tt.message).Error()
		if got != tt.want {
			t.Errorf("Wrap(%v, %q): got: %v, want %v", tt.err, tt.message, got, tt.want)
		}
	}
}

type nilError struct{}

func (nilError) Error() string { return "nil error" }

func TestWrapfNil(t *testing.T) {
	got := Wrapf(nil, "no error")
	if got != nil {
		t.Errorf("Wrapf(nil, \"no error\"): got %#v, expected nil", got)
	}
}
func TestWrapf(t *testing.T) {
	tests := []struct {
		err     error
		message string
		want    string
	}{
		{io.EOF, "read error", "read error: EOF"},
		{Wrapf(io.EOF, "read error without format specifiers"), "client error", "client error: read error without format specifiers: EOF"},
		{Wrapf(io.EOF, "read error with %d format specifier", 1), "client error", "client error: read error with 1 format specifier: EOF"},
	}

	for _, tt := range tests {
		got := Wrapf(tt.err, tt.message).Error()
		if got != tt.want {
			t.Errorf("Wrapf(%v, %q): got: %v, want %v", tt.err, tt.message, got, tt.want)
		}
	}
}

func TestErrorf(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{Errorf("read error without format specifiers"), "read error without format specifiers"},
		{Errorf("read error with %d format specifier", 1), "read error with 1 format specifier"},
	}

	for _, tt := range tests {
		got := tt.err.Error()
		if got != tt.want {
			t.Errorf("Errorf(%v): got: %q, want %q", tt.err, got, tt.want)
		}
	}
}

// errors.New, etc values are not expected to be compared by value
// but the change in errors#27 made them incomparable. Assert that
// various kinds of errors have a functional equality operator, even
// if the result of that equality is always false.
// ! YakError uncomparable because has originErorrs, is []error type.
// func TestErrorEquality(t *testing.T) {
// 	vals := []error{
// 		nil,
// 		io.EOF,
// 		errors.New("EOF"),
// 		Error("EOF"),
// 		Errorf("EOF"),
// 		Wrap(io.EOF, "EOF"),
// 		Wrapf(io.EOF, "EOF%d", 2),
// 	}

// 	for i := range vals {
// 		for j := range vals {
// 			_ = vals[i] == vals[j] // mustn't panic
// 		}
// 	}
// }

func TestJoin(t *testing.T) {
	t.Run("two-errors", func(t *testing.T) {
		err := io.EOF
		err = JoinErrors(err, io.ErrUnexpectedEOF)
		if !errors.Is(err, io.EOF) {
			t.Errorf("expected %v to contain %v", err, io.EOF)
		}

		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("expected %v to contain %v", err, io.ErrUnexpectedEOF)
		}
	})

	t.Run("nil", func(t *testing.T) {
		var err error = nil
		err = JoinErrors(err, nil)
		if !errors.Is(err, nil) {
			t.Errorf("expected nil but got %v", err)
		}
	})

}

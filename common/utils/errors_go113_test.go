package utils

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"testing"
)

func TestErrorChainCompat(t *testing.T) {
	err := stderrors.New("error that gets wrapped")
	wrapped := Wrap(err, "wrapped up")
	if !stderrors.Is(wrapped, err) {
		t.Errorf("Wrap does not support Go 1.13 error chains")
	}
}

func TestIs(t *testing.T) {
	err := Error("test")
	err2 := Error("test2")

	type args struct {
		err    error
		target error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "raw",
			args: args{
				err:    err,
				target: err,
			},
			want: true,
		},
		{
			name: "wrap",
			args: args{
				err:    Wrap(err, "test"),
				target: err,
			},
			want: true,
		},
		{
			name: "wrap-double",
			args: args{
				err:    Wrap(Wrap(err, "test"), "test"),
				target: err,
			},
			want: true,
		},
		{
			name: "with message format",
			args: args{
				err:    Wrapf(err, "%s", "test"),
				target: err,
			},
			want: true,
		},
		{
			name: "std errors compatibility",
			args: args{
				err:    fmt.Errorf("wrap it: %w", err),
				target: err,
			},
			want: true,
		},
		{
			name: "join-errors-err1",
			args: args{
				err:    JoinErrors(err, err2),
				target: err,
			},
			want: true,
		},
		{
			name: "join-errors-err2",
			args: args{
				err:    JoinErrors(err, err2),
				target: err2,
			},
			want: true,
		},
		{
			name: "negative",
			args: args{
				err:    err,
				target: err2,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stderrors.Is(tt.args.err, tt.args.target); got != tt.want {
				t.Errorf("Is() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testErr struct {
	msg string
}

func (c testErr) Error() string { return c.msg }

func TestAs(t *testing.T) {
	err := testErr{msg: "test message"}

	type args struct {
		err    error
		target interface{}
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "wrap",
			args: args{
				err:    Wrap(err, "test"),
				target: new(testErr),
			},
			want: true,
		},
		{
			name: "wrap format",
			args: args{
				err:    Wrapf(err, "%s", "test"),
				target: new(testErr),
			},
			want: true,
		},
		{
			name: "std errors compatibility",
			args: args{
				err:    fmt.Errorf("wrap it: %w", err),
				target: new(testErr),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stderrors.As(tt.args.err, tt.args.target); got != tt.want {
				t.Errorf("As() = %v, want %v", got, tt.want)
			}

			ce := tt.args.target.(*testErr)
			if !reflect.DeepEqual(err, *ce) {
				t.Errorf("set target error failed, target error is %v", *ce)
			}
		})
	}
}

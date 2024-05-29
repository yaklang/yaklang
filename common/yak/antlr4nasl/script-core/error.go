package script_core

type errorString struct {
	s string
}

func (e errorString) Error() string {
	return e.s
}

var (
	requirements_error = errorString{"requirements_error"}
)

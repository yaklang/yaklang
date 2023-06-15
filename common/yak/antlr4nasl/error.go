package antlr4nasl

type multiError []error

func (m multiError) Error() string {
	var s string
	for _, err := range m {
		s += err.Error() + "\n"
	}
	return s
}

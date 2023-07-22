package fuzztagx

type FuzzTag struct {
	label  string
	Method []FuzzTagMethod
}
type FuzzTagMethod struct {
	name  string
	param string
}

func (f *FuzzTag) Exec() []string {
	return nil
}

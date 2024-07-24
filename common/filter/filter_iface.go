package filter

type Filterable interface {
	Exist(str string) bool
	Insert(str string) bool
	Close()
	Clear()
}

var _ Filterable = &MapFilter{}
var _ Filterable = &StringFilter{}

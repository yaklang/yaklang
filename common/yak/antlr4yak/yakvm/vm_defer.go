package yakvm

type Defer struct {
	Codes []*Code
	Scope *Scope
}

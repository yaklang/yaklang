package yakvm

type CompilerWrapperInterface interface {
	NewWithSymbolTable(*SymbolTable) CompilerWrapperInterface
	Compiler(code string) bool
	GetNormalErrors() (bool, error)
	GetOpcodes() []*Code
}

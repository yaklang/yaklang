package values

const (
	ADD  = "add"
	INC  = "inc"
	New  = "new"
	NEQ  = "!="
	EQ   = "=="
	LT   = "<"
	GTE  = ">="
	GT   = ">"
	NE   = "!="
	Not  = "!"
	LTE  = "<="
	SUB  = "-"
	REM  = "%"
	DIV  = "/"
	MUL  = "*"
	AND  = "&"
	OR   = "|"
	XOR  = "^"
	SHL  = "<<"
	SHR  = ">>"
	USHR = ">>>"

	LOGICAL_AND = "&&"
	LOGICAL_OR  = "||"
)

func IsLogicalOperator(op string) bool {
	return op == LOGICAL_AND || op == LOGICAL_OR || op == "&" || op == "|" || op == "^" || op == "!"
}

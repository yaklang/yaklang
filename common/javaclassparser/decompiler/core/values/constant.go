package values

const (
	ADD  = "add"
	INC  = "inc"
	DEC  = "dec"
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

// IsStrictBooleanOperator reports whether op is unconditionally boolean-valued and
// boolean-typed in its operands. The bitwise operators &, |, ^ are deliberately excluded:
// the JVM shares IAND/IOR/IXOR between boolean logic and integer bitwise arithmetic, so the
// operand type must be taken from the values themselves (descriptor-seeded) rather than forced
// to boolean. Forcing it turned `int r = a & b` into `boolean r = a & b`, which then failed to
// compile against the following `r << 2`/`return r` integer uses.
func IsStrictBooleanOperator(op string) bool {
	return op == LOGICAL_AND || op == LOGICAL_OR || op == Not
}

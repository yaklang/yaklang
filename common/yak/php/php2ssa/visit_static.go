//go:build !no_language
// +build !no_language

package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitStaticVariableStatement(raw phpparser.IStaticVariableStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.StaticVariableStatementContext)
	if i == nil {
		return nil
	}
	for _, initializerContext := range i.AllVariableInitializer() {
		initializer, value := y.VisitVariableInitializer(initializerContext)
		y.AssignVariable(y.CreateVariable(initializer), value)
	}
	return nil
}

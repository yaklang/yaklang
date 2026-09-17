package stream_parser

import "github.com/yaklang/yaklang/common/bin-parser/parser/base"

// Expression evaluation temporarily removes its own expression from the current
// config to obtain the unformatted result. That removal does not change the
// assignment replay journal, so an unchanged config needs only a current-value
// restore. Recording it again would retain one closure per Result invocation.
type expressionConfigHistory struct {
	config     *base.Config
	count      int
	present    bool
	wellFormed bool
}

func captureExpressionConfigHistory(node *base.Node) expressionConfigHistory {
	state := expressionConfigHistory{config: node.Cfg}
	state.count, state.present, state.wellFormed = node.Cfg.ReplayHistoryLen()
	return state
}

func (before expressionConfigHistory) restore(node *base.Node, key, code string) {
	after := captureExpressionConfigHistory(node)
	if before.config == after.config && before.present == after.present &&
		before.wellFormed && after.wellFormed && before.count == after.count {
		node.Cfg.BaseKV.SetItem(key, code)
		return
	}
	// A custom expression can explicitly change configuration, including its
	// own expression, delete it again, or replace the Config. Preserve the old
	// final assignment in that case: future alias/import replay must still see
	// the restored expression after those newly recorded assignments. Ordinary
	// Config.SetItem only appends; raw mutation of the private replay journal is
	// not an expression contract. Do not change generic Config/Copy semantics.
	node.Cfg.SetItem(key, code)
}

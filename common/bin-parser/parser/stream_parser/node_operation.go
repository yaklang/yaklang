package stream_parser

import "github.com/yaklang/yaklang/common/bin-parser/parser/base"

// The public callback-based Operator remains usable by custom callers and
// generators. Built-in parsing implements the same operations with explicit
// invocation state, without constructing a set of closures for every node.
type nodeOperation interface {
	mode() string
	parseStruct(*base.Node) (bool, error)
	parseTerminal(*base.Node) error
	parseNode(*base.Node) error
	backup() error
	recovery() error
	popBackup() error
}

func (o *Operator) mode() string { return o.Mode }
func (o *Operator) parseStruct(n *base.Node) (bool, error) {
	if o.ParseStruct == nil {
		return false, nil
	}
	return o.ParseStruct(n)
}
func (o *Operator) parseTerminal(n *base.Node) error { return o.ParseTerminal(n) }
func (o *Operator) parseNode(n *base.Node) error     { return o.NodeParse(n) }
func (o *Operator) backup() error                    { return o.Backup() }
func (o *Operator) recovery() error                  { return o.Recovery() }
func (o *Operator) popBackup() error                 { return o.PopBackup() }

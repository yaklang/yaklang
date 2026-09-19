package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// These private operations are used only by closed plans whose discarded
// return values cannot escape. The public YakNode and ordinary VM method
// results retain their full mutable values and response maps.
func (n *operatorNode) processDiscard() {
	finish, err := n.operator(n.origin)
	if err != nil {
		if finish != nil {
			finish(true)
		}
		panic(err)
	}
	finish(false)
	if err := n.origin.ValidateResult(); err != nil {
		panic(err)
	}
}

func (n *operatorNode) processSubNodeDiscard(name string) {
	n.GetSubNode(name).processDiscard()
}

func (n *operatorNode) processByTypeDiscard(name string) {
	template := n.getRootNode(name)
	n.AppendNode(template)
	target := n.origin.Children[len(n.origin.Children)-1]
	target.Name = name
	convertOperatorNode(target, n.operator).processDiscard()
}

type discardedTrial struct {
	ok        bool
	message   string
	parent    *base.Node
	finish    func(bool)
	installed bool
}

func (t *discardedTrial) save() {
	if t.installed {
		t.finish(false)
	}
}

func (t *discardedTrial) recovery() {
	if t.installed {
		t.finish(true)
		t.parent.Children = t.parent.Children[:len(t.parent.Children)-1]
	}
}

func (n *operatorNode) tryProcessByTypeDiscard(name string) (trial discardedTrial) {
	// In the original method, root lookup precedes its local recover handler.
	// Preserve that distinction and install recovery only after operator returns.
	template := n.getRootNode(name)
	trial.parent = n.origin
	defer func() {
		if recovered := recover(); recovered != nil {
			trial.message = fmt.Sprintf("%v", recovered)
		}
	}()
	n.AppendNode(template)
	target := n.origin.Children[len(n.origin.Children)-1]
	if name != "" {
		target.Name = name
	}
	finish, err := n.operator(target)
	trial.finish, trial.installed = finish, true
	if err != nil {
		trial.message = err.Error()
		return
	}
	if err := target.ValidateResult(); err != nil {
		panic(err)
	}
	trial.ok = true
	return
}

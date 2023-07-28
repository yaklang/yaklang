package ssa

func (b *BasicBlock) AddSucc(succ *BasicBlock) {
	b.Succs = append(b.Succs, succ)
	succ.Preds = append(succ.Preds, b)
}

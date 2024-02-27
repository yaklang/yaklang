package ssa

func RunOnCoverOr[T, U Instruction](insts []U, cover func(Instruction) (T, bool), f func(T), or func(U)) {
	for _, inst := range insts {
		if t, ok := cover(inst); ok {
			f(t)
		} else {
			or(inst)
		}
	}
}

func noneForUser(User) {}

func (u Users) RunOnCallOr(f func(*Call), or func(User)) {
	RunOnCoverOr(
		u,
		ToCall, f, or,
	)
}
func (u Users) RunOnCall(f func(*Call)) {
	u.RunOnCallOr(f, noneForUser)
}

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

func (u Users) RunOnFieldOr(f func(*Field), or func(User)) {
	RunOnCoverOr(
		u,
		ToField, f, or,
	)
}
func (u Users) RunOnField(f func(*Field)) {
	u.RunOnFieldOr(f, noneForUser)
}

func (u Users) RunOnUpdateOr(f func(*Update), or func(User)) {
	RunOnCoverOr(
		u,
		ToUpdate, f, or,
	)
}
func (u Users) RunOnUpdate(f func(*Update)) {
	u.RunOnUpdateOr(f, noneForUser)
}

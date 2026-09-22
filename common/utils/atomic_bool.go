package utils

import "github.com/yaklang/yaklang/common/utils/atomicbool"

// AtomicBool is an atomic Boolean. Use a pointer; do not copy after first use.
type AtomicBool = atomicbool.AtomicBool

func NewAtomicBool() *AtomicBool  { return atomicbool.New() }
func NewBool(ok bool) *AtomicBool { return atomicbool.NewBool(ok) }

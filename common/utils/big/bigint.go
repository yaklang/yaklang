package big

import "math/big"

type BigInt struct {
	*big.Int
}

func NewInt(i int64) *BigInt {
	return &BigInt{big.NewInt(i)}
}

func NewFromString(s string, base int) *BigInt {
	i, _ := new(big.Int).SetString(s, base)
	return &BigInt{i}
}

func NewDecFromString(s string) *BigInt {
	i, _ := new(big.Int).SetString(s, 10)
	return &BigInt{i}
}

func (b *BigInt) Add(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Add(b.Int, i.Int)}
}

func (b *BigInt) AddInt(i int64) *BigInt {
	return &BigInt{new(big.Int).Add(b.Int, big.NewInt(i))}
}

func (b *BigInt) Sub(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Sub(b.Int, i.Int)}
}

func (b *BigInt) SubInt(i int64) *BigInt {
	return &BigInt{new(big.Int).Sub(b.Int, big.NewInt(i))}
}

func (b *BigInt) Mul(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Mul(b.Int, i.Int)}
}

func (b *BigInt) Div(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Div(b.Int, i.Int)}
}

func (b *BigInt) DivMod(i *BigInt) (*BigInt, *BigInt) {
	div, mod := new(big.Int).DivMod(b.Int, i.Int, new(big.Int))
	return &BigInt{div}, &BigInt{mod}
}

func (b *BigInt) Quo(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Quo(b.Int, i.Int)}
}

func (b *BigInt) QuoRem(i *BigInt) (*BigInt, *BigInt) {
	quo, rem := new(big.Int).QuoRem(b.Int, i.Int, new(big.Int))
	return &BigInt{quo}, &BigInt{rem}
}

func (b *BigInt) Mod(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Mod(b.Int, i.Int)}
}

func (b *BigInt) Pow(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Exp(b.Int, i.Int, nil)}
}

func (b *BigInt) Exp(i, m *BigInt) *BigInt {
	return &BigInt{new(big.Int).Exp(b.Int, i.Int, m.Int)}
}

func (b *BigInt) And(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).And(b.Int, i.Int)}
}

func (b *BigInt) Or(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Or(b.Int, i.Int)}
}

func (b *BigInt) Xor(i *BigInt) *BigInt {
	return &BigInt{new(big.Int).Xor(b.Int, i.Int)}
}

func (b *BigInt) Lsh(i uint) *BigInt {
	return &BigInt{new(big.Int).Lsh(b.Int, i)}
}

func (b *BigInt) Rsh(i uint) *BigInt {
	return &BigInt{new(big.Int).Rsh(b.Int, i)}
}

func (b *BigInt) Neg() *BigInt {
	return &BigInt{new(big.Int).Neg(b.Int)}
}

func (b *BigInt) Abs() *BigInt {
	return &BigInt{new(big.Int).Abs(b.Int)}
}

func (b *BigInt) Copy() *BigInt {
	return &BigInt{new(big.Int).Set(b.Int)}
}

func (b *BigInt) Greater(i *BigInt) bool {
	return b.Int.Cmp(i.Int) == 1
}

func (b *BigInt) Less(i *BigInt) bool {
	return b.Int.Cmp(i.Int) == -1
}

func (b *BigInt) Equal(i *BigInt) bool {
	return b.Int.Cmp(i.Int) == 0
}

func (b *BigInt) GreatOrEqual(i *BigInt) bool {
	return b.Int.Cmp(i.Int) >= 0
}

func (b *BigInt) LessOrEqual(i *BigInt) bool {
	return b.Int.Cmp(i.Int) <= 0
}

func (b *BigInt) Cmp(i *BigInt) int {
	return b.Int.Cmp(i.Int)
}

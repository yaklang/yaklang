package core

import "strings"

// CompareOp is the comparison one condition code selects.
type CompareOp int

const (
	CompareEQ CompareOp = iota
	CompareNE
	CompareLT
	CompareLE
	CompareGT
	CompareGE
)

// Condition is one decoded condition code: the comparison it selects and
// whether the operands are signed. Targets print the same comparison with
// different mnemonics, so a lifter keeps the mnemonic its architecture prints and
// the converter decodes it here.
type Condition struct {
	Op     CompareOp
	Signed bool
}

// conditions maps every mnemonic a target prints to the comparison it selects.
// Equality ignores signedness, so those entries leave Signed false.
//
// The spellings come from the three families the loaders lift:
//   - x86 prints e/z, ne/nz, l/le/g/ge for signed and b/be/a/ae for unsigned, plus
//     the carry aliases c/nc and the negated forms nae/na/nbe/nb;
//   - AArch64 prints eq, ne, lt/le/gt/ge, cs/hs, cc/lo, hi/ls and the sign
//     conditions mi/pl;
//   - s390x selects a name from the condition mask: e, ne, l/le, g/ge, lo/hs,
//     hi/ls.
//
// Overflow conditions (o, no, vs, vc) name a flag YakSSA has no value for, so
// they are absent: ParseCondition reports them as unknown and the converter
// leaves the condition opaque instead of inventing a comparison.
var conditions = map[string]Condition{
	"z":  {Op: CompareEQ},
	"e":  {Op: CompareEQ},
	"eq": {Op: CompareEQ},
	"nz": {Op: CompareNE},
	"ne": {Op: CompareNE},

	"l":   {Op: CompareLT, Signed: true},
	"lt":  {Op: CompareLT, Signed: true},
	"nge": {Op: CompareLT, Signed: true},
	"s":   {Op: CompareLT, Signed: true},
	"mi":  {Op: CompareLT, Signed: true},
	"le":  {Op: CompareLE, Signed: true},
	"ng":  {Op: CompareLE, Signed: true},
	"g":   {Op: CompareGT, Signed: true},
	"gt":  {Op: CompareGT, Signed: true},
	"nle": {Op: CompareGT, Signed: true},
	"ge":  {Op: CompareGE, Signed: true},
	"pl":  {Op: CompareGE, Signed: true},
	"ns":  {Op: CompareGE, Signed: true},

	"b":   {Op: CompareLT},
	"c":   {Op: CompareLT},
	"nae": {Op: CompareLT},
	"lo":  {Op: CompareLT},
	"cc":  {Op: CompareLT},
	"be":  {Op: CompareLE},
	"na":  {Op: CompareLE},
	"ls":  {Op: CompareLE},
	"a":   {Op: CompareGT},
	"nbe": {Op: CompareGT},
	"hi":  {Op: CompareGT},
	"ae":  {Op: CompareGE},
	"cs":  {Op: CompareGE},
	"nb":  {Op: CompareGE},
	"nc":  {Op: CompareGE},
	"hs":  {Op: CompareGE},
}

// ParseCondition decodes one condition mnemonic, ignoring case and surrounding
// space.
func ParseCondition(name string) (Condition, bool) {
	condition, ok := conditions[strings.ToLower(strings.TrimSpace(name))]
	return condition, ok
}

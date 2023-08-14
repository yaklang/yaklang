package ssa

import (
	"go/types"
	"runtime"
	"strings"

	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

type Type types.Type
type Types []Type

var (
	basicTypes = make(map[string]*types.Basic)
)

func init() {
	for _, basic := range types.Typ {
		basicTypes[basic.Name()] = basic
	}
	if strings.Contains(runtime.GOARCH, "64") {
		basicTypes["int"] = basicTypes["int64"]
	} else {
		basicTypes["int"] = basicTypes["int32"]
	}
}

// return true  if org != typs
// return false if org == typs
func (org Types) Compare(typs Types) bool {
	if len(org) == 0 && len(typs) != 0 {
		return true
	}
	return slices.CompareFunc(org, typs, func(org, typ Type) int {
		if types.Identical(org, typ) {
			return 0
		}
		return 1
	}) != 0
}

func (t Types) String() string {
	return strings.Join(
		lo.Map(t, func(typ Type, _ int) string {
			if typ == nil {
				return "nil"
			} else {
				return typ.String()
			}
		}),
		", ",
	)
}

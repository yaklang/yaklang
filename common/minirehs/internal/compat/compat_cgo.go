//go:build cgo

// Package compat keeps the native Yak verifier and value conversions out of
// the dependency graph of portable, non-cgo minirehs applications.
package compat

import (
	"github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

type YakRegexpUtils = regexp_utils.YakRegexpUtils

func NewYakRegexpUtils(expr string) *YakRegexpUtils { return regexp_utils.NewYakRegexpUtils(expr) }

var InterfaceToBytes = utils.InterfaceToBytes
var InterfaceToStringSlice = utils.InterfaceToStringSlice

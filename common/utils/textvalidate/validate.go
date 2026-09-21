// Package textvalidate preserves the lexical predicates used by Yak.
// Derived from asaskevich/govalidator f21760c49a8d; see LICENSE.
package textvalidate

import "regexp"

var (
	rxInt            = regexp.MustCompile("^(?:[-+]?(?:0|[1-9][0-9]*))$")
	rxFloat          = regexp.MustCompile("^(?:[-+]?(?:[0-9]+))?(?:\\.[0-9]*)?(?:[eE][\\+\\-]?(?:[0-9]+))?$")
	rxBase64         = regexp.MustCompile("^(?:[A-Za-z0-9+\\/]{4})*(?:[A-Za-z0-9+\\/]{2}==|[A-Za-z0-9+\\/]{3}=|[A-Za-z0-9+\\/]{4})$")
	rxPrintableASCII = regexp.MustCompile("^[\x20-\x7E]+$")
)

func IsInt(str string) bool {
	if str == "" {
		return true
	}
	return rxInt.MatchString(str)
}

func IsFloat(str string) bool {
	return str != "" && rxFloat.MatchString(str)
}

func IsBase64(str string) bool {
	return rxBase64.MatchString(str)
}

func IsPrintableASCII(str string) bool {
	if str == "" {
		return true
	}
	return rxPrintableASCII.MatchString(str)
}

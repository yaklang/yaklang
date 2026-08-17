package aotlib

import "strings"

func StrSplit(s, sep string) []string { return strings.Split(s, sep) }
func StrTrim(s, cutset string) string { return strings.Trim(s, cutset) }
func StrTrimSpace(s string) string    { return strings.TrimSpace(s) }
func StrJoin(parts []string, sep string) string {
	return strings.Join(parts, sep)
}
func StrToLower(s string) string         { return strings.ToLower(s) }
func StrToUpper(s string) string         { return strings.ToUpper(s) }
func StrHasPrefix(s, prefix string) bool { return strings.HasPrefix(s, prefix) }
func StrHasSuffix(s, suffix string) bool { return strings.HasSuffix(s, suffix) }
func StrContains(s, substr string) bool  { return strings.Contains(s, substr) }
func StrReplace(s, old, new string, n int) string {
	return strings.Replace(s, old, new, n)
}
func StrRepeat(s string, count int) string { return strings.Repeat(s, count) }
func StrIndex(s, substr string) int        { return strings.Index(s, substr) }
func StrLen(s string) int                  { return len(s) }

// StringsExports mirrors the str module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.StringsExport signatures.
var StringsExports = map[string]any{
	"Split":     StrSplit,
	"Trim":      StrTrim,
	"TrimSpace": StrTrimSpace,
	"Join":      StrJoin,
	"ToLower":   StrToLower,
	"ToUpper":   StrToUpper,
	"HasPrefix": StrHasPrefix,
	"HasSuffix": StrHasSuffix,
	"Contains":  StrContains,
	"Replace":   StrReplace,
	"Repeat":    StrRepeat,
	"Index":     StrIndex,
	"Len":       StrLen,
}

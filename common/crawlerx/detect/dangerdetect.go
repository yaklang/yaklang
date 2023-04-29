package detect

import "strings"

func NormalCheckDangerUrl(Sensitives []string) func(string) bool {
	return func(s string) bool {
		for _, word := range Sensitives {
			if strings.Contains(s, word) {
				return true
			}
		}
		return false
	}
}

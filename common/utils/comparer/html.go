package comparer

import "yaklang.io/yaklang/common/utils/bs4"

func CompareHtml(s1, s2 []byte) (f float64) {
	f = compareBytes(s1, s2)

	defer func() {
		if err := recover(); err != nil {

		}
	}()

	b1 := bs4.HTMLParse(string(s1))
	b2 := bs4.HTMLParse(string(s2))
	f = f * 0.5
	return float64(score(f).Add(compareString(b1.FullText(), b2.FullText()), 0.5))
}

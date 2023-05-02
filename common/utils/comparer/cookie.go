package comparer

import (
	"net/http"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

func compareCookies(c1, c2 []*http.Cookie) float64 {
	var keysTotal []string
	var m1, m2 = make(map[string]*http.Cookie), make(map[string]*http.Cookie)
	for _, c := range c1 {
		m1[c.Name] = c
		if !utils.StringSliceContain(keysTotal, c.Name) {
			keysTotal = append(keysTotal, c.Name)
		}
	}

	for _, c := range c2 {
		m2[c.Name] = c
		if !utils.StringSliceContain(keysTotal, c.Name) {
			keysTotal = append(keysTotal, c.Name)
		}
	}

	var results []float64
	for _, k := range keysTotal {
		var v1, v2 string
		m1v, ok := m1[k]
		if ok {
			v1 = m1v.String()
		}
		m2v, ok := m2[k]
		if ok {
			v2 = m2v.String()
		}

		results = append(results, compareString(v1, v2))
	}
	if len(results) > 0 {
		return funk.SumFloat64(results) / float64(len(results))
	}
	return 1
}

// Package httpbrute
// @Author bcy2007  2023/6/20 16:15
package httpbrute

import "github.com/yaklang/yaklang/common/simulator/core"

func ElementsFilter(elements *core.GeneralElements, keyword string) *core.GeneralElement {
	tempElements := elements.FilteredKeywordElements(keyword)
	if tempElements.Single() {
		return tempElements.First()
	} else if tempElements.Multi() {
		return tempElements.FilteredKeywordElement(keyword)
	} else {
		return nil
	}
}

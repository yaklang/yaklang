// Package simulator
// @Author bcy2007  2023/8/17 17:28
package simulator

import (
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
	"strings"
)

func CalculateRelevanceMatrix(elements rod.Elements, elementTypes []string) (map[string]*rod.Element, error) {
	result := make(map[string]*rod.Element)
	keys := maps.Keys(KeywordDict)
	if !ArrayInArray(elementTypes, keys) {
		return result, utils.Error(`detect type more than exist type`)
	}
	matrix := make([][]float64, 0)
	for _, element := range elements {
		outer, err := ElementToValue(element, `()=>this.outerHTML`)
		if err != nil {
			return result, utils.Error(err)
		}
		elementVector := make([]float64, 0)
		for _, elementType := range elementTypes {
			// least words check
			simpleElementTypeList, ok := SimpleKeywordDict[elementType]
			if !ok {
				return result, utils.Errorf(`%v not exist`, elementType)
			}
			if !ArrayStringContains(simpleElementTypeList, outer) {
				elementVector = append(elementVector, -1)
				continue
			}
			// relevance degree calculate
			relevance := CalculateRelevance(element, elementType)
			elementVector = append(elementVector, relevance)
		}
		matrix = append(matrix, elementVector)
	}
	//selectors := ElementsToSelectors(elements...)
	//items := ElementsToValue(elements, `()=>this.outerHTML`)
	m := DataMatrix[*rod.Element]{
		ItemList: elements,
		TagList:  elementTypes,
		Data:     matrix,
	}
	err := m.ValidCheck()
	if err != nil {
		//log.Info(selectors, elementTypes, matrix, len(selectors))
		return result, utils.Error("matrix valid check failed")

	}
	result, err = m.GetResult()
	if err != nil {
		return result, utils.Error(err)
	}
	//log.Infof("result: %v", result)
	return result, nil
}

func CalculateRelevance(element *rod.Element, elementType string) float64 {
	keywords, ok := KeywordDict[elementType]
	if !ok {
		log.Errorf(`%v in keywords dict not found`, elementType)
		return 0
	}
	var likeValue float64
	for _, attr := range ElementKeyword {
		attribute, err := GetElementParam(element, attr)
		if err != nil || attribute == "" {
			continue
		}
		tempValue := calculateGroupRelevance(attribute, keywords)
		if tempValue > likeValue {
			likeValue = tempValue
		}
	}
	return likeValue
}

func calculateGroupRelevance(target string, originGroup []string) (likeValue float64) {
	target = strings.ToLower(target)
	for _, origin := range originGroup {
		tempValue := calculateRelevance(target, origin)
		if tempValue > likeValue {
			likeValue = tempValue
		}
	}
	return
}

func calculateRelevance(target, source string) float64 {
	repeatStr := GetRepeatStr(target, source)
	return float64(len(repeatStr)) / float64(utils.Max(len(target), len(source)))
}

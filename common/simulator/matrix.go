// Package simulator
// @Author bcy2007  2023/8/17 16:20
package simulator

import (
	"github.com/yaklang/yaklang/common/utils"
)

type DataMatrix struct {
	ItemList []string
	TagList  []string
	Data     [][]float64
}

func (matrix *DataMatrix) ValidCheck() error {
	if len(matrix.Data) != len(matrix.ItemList) {
		return utils.Errorf(`data items number: %v, item list length: %v`, len(matrix.Data), len(matrix.ItemList))
	}
	tagLength := len(matrix.TagList)
	for _, d := range matrix.Data {
		if len(d) != tagLength {
			return utils.Errorf(`data tags number: %v, tag list length: %v`, len(d), tagLength)
		}
	}
	return nil
}

func (matrix *DataMatrix) GetResult() (map[string]string, error) {
	result := make(map[string]string)
	var tempData = matrix.Data
	var tempItem = matrix.ItemList
	var tempTag = matrix.TagList
	var num int
	for len(tempData) != 0 {
		num++
		var maxRow, maxColumn int
		var maxData float64 = -100
		for row, items := range tempData {
			column, tempMax := getMax(items)
			if tempMax > maxData {
				maxRow = row
				maxColumn = column
				maxData = tempMax
			}
		}

		afterRemove := make([][]float64, 0)
		for row, rows := range tempData {
			if row == maxRow {
				continue
			}
			temp := append(rows[:maxColumn], rows[maxColumn+1:]...)
			if len(temp) != 0 {
				afterRemove = append(afterRemove, temp)
			}
		}
		if maxData <= 0 {
			result[tempTag[maxColumn]] = ""
		} else {
			result[tempTag[maxColumn]] = tempItem[maxRow]
		}
		tempItem = append(tempItem[:maxRow], tempItem[maxRow+1:]...)
		tempTag = append(tempTag[:maxColumn], tempTag[maxColumn+1:]...)
		tempData = afterRemove
		//info := fmt.Sprintf("round %d result: %v %v %v %v", num, tempTag, tempItem, tempData, result)
		//log.Info(info)
	}
	return result, nil
}

func getMax(data []float64) (int, float64) {
	var maxData float64 = -100
	var maxPosition int
	for pos, item := range data {
		if item > maxData {
			maxData = item
			maxPosition = pos
		}
	}
	return maxPosition, maxData
}

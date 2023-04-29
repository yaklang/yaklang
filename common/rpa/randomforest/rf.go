package randomforest

import (
	"fmt"
	"yaklang/common/log"
	"yaklang/common/rpa/character"
	"yaklang/common/utils"

	"github.com/fxsjy/RF.go/RF"
)

type UrlDetectSys struct {
	X        [][]interface{}
	Y        []string
	Filepath string
	model    *RF.Forest
}

func (sys *UrlDetectSys) SysReadFile() {
	_x, _y, err := ReadFile(sys.Filepath)
	if err != nil {
		log.Errorf("read data file error: %s", err)
		return
	}
	sys.X = _x
	sys.Y = _y
}

func (sys *UrlDetectSys) RebuildData(splitNum int) ([][]interface{}, []string) {
	positive, negative := SplitDatafromY(sys.X, sys.Y)
	posNum := len(positive)
	negaNum := len(negative)
	posNums, _ := RandomNumberGenerate(0, posNum, splitNum)
	negaNums, _ := RandomNumberGenerate(0, negaNum, splitNum)
	var alllastX [][]interface{}
	var alllasty []string
	for _, num := range posNums {
		alllastX = append(alllastX, positive[num])
		alllasty = append(alllasty, "1")
	}
	for _, num := range negaNums {
		alllastX = append(alllastX, negative[num])
		alllasty = append(alllasty, "0")
	}
	return alllastX, alllasty
}

func (sys *UrlDetectSys) SysTrain(x [][]interface{}, y []string) {
	forest := RF.BuildForest(x, y, 80, 1200, len(x[0]))
	sys.model = forest
}

func (sys *UrlDetectSys) PredictScore(xx [][]interface{}, yy []string) {
	error_count := 0.0
	for i := 0; i < len(xx); i++ {
		output := sys.model.Predicate(xx[i])
		expected := yy[i]
		if output != expected {
			fmt.Println(output, " ", expected)
			error_count += 1
		} else {
			fmt.Println("***", output, " ", expected)
		}
	}
	fmt.Println("success rate:", 1.0-error_count/float64(len(xx)))
}

func (sys *UrlDetectSys) PredictX(s string) string {
	ss := character.String2Vec(s)
	output := sys.model.Predicate(ss)
	return output
}

func (sys *UrlDetectSys) DumpModel(path string) error {
	if sys.model == nil {
		return utils.Errorf("Empty Model")
	}
	RF.DumpForest(sys.model, path)
	return nil
}

func (sys *UrlDetectSys) LoadModel(path string) error {
	forest := RF.LoadForest(path)
	sys.model = forest
	return nil
}

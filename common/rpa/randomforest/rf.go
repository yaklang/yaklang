package randomforest

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa/character"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/fxsjy/RF.go/RF"
)

// rf.model.gz 是 rf.model 的 gzip 副本（2.4MB -> 76KB），随二进制一起发布。
// 重新生成：gzip -9 -n -c rf.model > rf.model.gz
//
//go:embed rf.model.gz
var embeddedRFModelGz []byte

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

// LoadModel 从磁盘路径加载模型，仅供本地调试/训练使用。
func (sys *UrlDetectSys) LoadModel(path string) error {
	forest := RF.LoadForest(path)
	sys.model = forest
	return nil
}

// LoadEmbeddedModel 加载编译进二进制的模型。
// 之前这里用的是一个写死的开发机绝对路径，除作者机器外都会 os.Stat 失败，
// 导致 strictUrl 实际上从来没有生效过。
func (sys *UrlDetectSys) LoadEmbeddedModel() error {
	raw, err := utils.GzipDeCompress(embeddedRFModelGz)
	if err != nil {
		return utils.Wrap(err, "decompress embedded rf model failed")
	}
	forest := &RF.Forest{}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(forest); err != nil {
		return utils.Wrap(err, "decode embedded rf model failed")
	}
	sys.model = forest
	return nil
}

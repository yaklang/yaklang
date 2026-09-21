package randomforest

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa/character"
	"github.com/yaklang/yaklang/common/rpa/randomforest/internal/rf"
	"github.com/yaklang/yaklang/common/utils"
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
	model    *rf.Forest
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
	forest := rf.BuildForest(x, y, 80, 1200, len(x[0]))
	sys.model = forest
}

// PredictScore returns classification accuracy, or zero for an empty sample set.
func (sys *UrlDetectSys) PredictScore(xx [][]interface{}, yy []string) float64 {
	if len(xx) == 0 {
		return 0
	}
	incorrect := 0
	for i, sample := range xx {
		if sys.model.Predicate(sample) != yy[i] {
			incorrect++
		}
	}
	return 1 - float64(incorrect)/float64(len(xx))
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
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(sys.model); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// LoadModel 从磁盘路径加载模型，仅供本地调试/训练使用。
func (sys *UrlDetectSys) LoadModel(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	forest := &rf.Forest{}
	if err := json.NewDecoder(f).Decode(forest); err != nil {
		return err
	}
	sys.model = forest
	return nil
}

// LoadEmbeddedModel 加载编译进二进制的模型。
func (sys *UrlDetectSys) LoadEmbeddedModel() error {
	raw, err := utils.GzipDeCompress(embeddedRFModelGz)
	if err != nil {
		return utils.Wrap(err, "decompress embedded rf model failed")
	}
	forest := &rf.Forest{}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(forest); err != nil {
		return utils.Wrap(err, "decode embedded rf model failed")
	}
	sys.model = forest
	return nil
}

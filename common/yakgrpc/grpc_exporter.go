package yakgrpc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExtractDataToFile(input ypb.Yak_ExtractDataToFileServer) error {
	var results []map[string]string
	var csvData [][]string
	var existedKeys = make(map[string]int)

	var jsonOutput = false
	var csvOutput = false
	var dirName string
	config, err := input.Recv()
	if err != nil {
		return utils.Errorf("first message is for config but met err: %s", err)
	}
	jsonOutput = config.GetJsonOutput()
	csvOutput = config.GetCSVOutput()
	dirName = config.GetDirName()
	if dirName == "" {
		dirName = consts.GetDefaultYakitBaseTempDir()
	}
	filePattern := config.GetFileNamePattern()
	if filePattern == "" {
		filePattern = fmt.Sprintf("yakit-output-*-%v", time.Now().Format(utils.DefaultTimeFormat))
	}

	if !jsonOutput && !csvOutput {
		return utils.Errorf("JsonOutput / CSVOutput should be selected at least one.")
	}

	for {
		result, err := input.Recv()
		if err != nil || result.GetFinished() {
			break
		}
		var data = result.GetData()
		if data == nil || len(data) <= 0 {
			// 排除空数据
			continue
		}
		for key := range data {
			_, ok := existedKeys[key]
			if !ok {
				existedKeys[key] = len(existedKeys)
			}
		}

		if csvOutput {
			// 处理 CSV 数据
			values := make([]string, len(existedKeys))
			for key, value := range data {
				if value == nil || (value.GetStringValue() == "" && len(value.GetBytesValue()) <= 0) {
					continue
				}
				if len(value.GetBytesValue()) > 0 {
					values[existedKeys[key]] = strings.ReplaceAll(utils.ParseStringToVisible(value.GetBytesValue()), ",", " ")
				} else {
					values[existedKeys[key]] = strings.ReplaceAll(utils.ParseStringToVisible(value.GetStringValue()), ",", " ")
				}
			}
			csvData = append(csvData, values)
		}

		if jsonOutput {
			// 处理 JSON 数据
			var jsonValue = make(map[string]string)
			for key, value := range data {
				bytes := value.GetBytesValue()
				if len(bytes) > 0 {
					jsonValue[key] = string(bytes)
					continue
				}
				jsonValue[key] = value.GetStringValue()
			}
			results = append(results, jsonValue)
		}
	}

	if jsonOutput {
		raw, err := json.MarshalIndent(results, "", "    ")
		if err != nil {
			return utils.Errorf("marshal json failed: %s", err)
		}
		fp, err := ioutil.TempFile(dirName, filePattern+".json")
		if err != nil {
			return utils.Errorf("open %v/%v.json failed: %s", dirName, filePattern, err)
		}
		fp.Write(raw)
		fp.Close()
		err = input.Send(&ypb.ExtractDataToFileResult{FilePath: fp.Name()})
		if err != nil {
			log.Errorf("exportor send back failed: %s", err)
		}
	}

	if csvOutput {

		fp, err := ioutil.TempFile(dirName, filePattern+".csv")
		if err != nil {
			return utils.Errorf("open %v/%v.json failed: %s", dirName, filePattern, err)
		}
		var header = make([]string, len(existedKeys))
		for value, index := range existedKeys {
			header[index] = value
		}
		fp.WriteString(fmt.Sprintf("%v\n", strings.Join(header, ",")))
		for _, value := range csvData {
			fp.WriteString(fmt.Sprintf("%v\n", strings.Join(value, ",")))
		}
		fp.Close()
		err = input.Send(&ypb.ExtractDataToFileResult{FilePath: fp.Name()})
		if err != nil {
			log.Errorf("exportor send back failed: %s", err)
		}
	}
	return nil
}

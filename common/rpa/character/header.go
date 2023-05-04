package character

import (
	"bufio"
	"encoding/json"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"strings"
)

func AnalysisHeaders(s string) (map[string]string, error) {
	if strings.Contains(s, "{") {
		return analysisHeadersfromJson(s)
	}
	return analysisHeadersfromFile(s)
}

func analysisHeadersfromFile(s string) (map[string]string, error) {
	if _, err := os.Stat(s); err != nil {
		return map[string]string{}, utils.Errorf("read header file error: %s", err)
	}
	fi, err := os.Open(s)
	if err != nil {
		return map[string]string{}, utils.Errorf("open files error:%s", err)
	}
	defer fi.Close()
	reader := bufio.NewReader(fi)
	h := make(map[string]string)
	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return map[string]string{}, utils.Errorf("read bytes error:%s", err)
		}
		if err == io.EOF {
			break
		}
		lineStr := string(lineBytes)
		if !strings.Contains(lineStr, ":") {
			continue
		}
		blocks := strings.Split(lineStr, ":")
		length := len(blocks)
		if length <= 1 {
			continue
		} else {
			h[blocks[0]] = strings.TrimSpace(strings.Join(blocks[1:], ":"))
		}
	}
	return h, nil
}

func analysisHeadersfromJson(s string) (map[string]string, error) {
	var formatted map[string]string
	err := json.Unmarshal([]byte(s), &formatted)
	if err != nil {
		// log.Info(err)
		return map[string]string{}, utils.Errorf("unmarshal headers json data error: %s", err)
	}
	return formatted, nil
}

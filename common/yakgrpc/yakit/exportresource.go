package yakit

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"os"
	"path/filepath"
	"strings"
	"yaklang/common/consts"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
)

func init() {
	// 自动加载以 .yakitresource.txt 结尾的数据
	RegisterPostInitDatabaseFunction(func() error {
		yakitResourceFile := filepath.Join(consts.GetDefaultYakitBaseDir(), "base")
		if utils.GetFirstExistedPath(yakitResourceFile) != "" {
			infos, _ := utils.ReadDir(yakitResourceFile)
			for _, i := range infos {
				if i.IsDir {
					continue
				}
				path := i.Path
				if strings.HasSuffix(path, ".yakitresource.txt") {
					log.Infof("start to import resource: %v", path)
					err := ImportResource(consts.GetGormProfileDatabase(), path)
					if err != nil {
						log.Error("import data[%v] failed: %v", path, err)
						continue
					}
					os.Rename(path, path+fmt.Sprintf(".%v.done", utils.RandStringBytes(20)))
				}

				if strings.HasSuffix(path, ".done") {
					log.Debugf("finished(existed) importing resource: %v", path)
				}
			}
		}
		return nil
	})
}

func ExportYakScript(db *gorm.DB, fileName string) error {
	fp, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer fp.Close()

	for result := range YieldYakScripts(db, context.Background()) {
		result.ID = 0
		data, err := json.Marshal(result)
		if err != nil {
			continue
		}
		raw, err := json.Marshal(map[string]string{
			"table":  "yak_scripts",
			"base64": codec.EncodeBase64(data),
		})
		if err != nil {
			continue
		}
		fp.Write(raw)
		fp.Write([]byte{'\n'})
	}
	return nil
}

func ImportResource(db *gorm.DB, fileName string) error {
	raw, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}

	for line := range utils.ParseLines(string(raw)) {
		var m = make(map[string]string)
		err := json.Unmarshal([]byte(line), &m)
		if err != nil {
			continue
		}
		switch ret, _ := m["table"]; ret {
		case "yak_scripts":
			d, _ := m["base64"]
			if d == "" {
				continue
			}
			decodedRaw, err := codec.DecodeBase64(d)
			if err != nil {
				continue
			}
			var s YakScript
			err = json.Unmarshal(decodedRaw, &s)
			if err != nil {
				log.Error(err)
				continue
			}
			sPtr := &s
			err = CreateOrUpdateYakScriptByName(db, sPtr.ScriptName, sPtr)
			if err != nil {
				log.Errorf("save script[%v] failed: %s", sPtr.ScriptName, err)
			}
		default:
			log.Warnf("cannot load table: %s", ret)
			continue
		}
	}
	return nil
}

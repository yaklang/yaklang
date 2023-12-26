package wsm

import (
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type GodzillaFileSystemAction struct {
	godzillaCache map[string]*Godzilla
}

// result
// ok
// /home
// COM 0 2023-09-13 22:05:47 16384 RWX
// Extensions 0 2023-09-12 20:46:13 4096 RWX
// unins000.dat 1 2023-09-12 20:46:35 135260 RWX
// unins000.exe 1 2023-09-12 20:44:43 953165 RWX
// WWW 0 2023-09-13 21:39:48 0 RWX
func godzillaResultToYakURLResource(originParam *ypb.YakURL, result []byte) ([]*ypb.YakURLResource, error) {
	// Split the result into lines.
	lines := strings.Split(string(result), "\n")
	if lines[0] != "ok" {
		return nil, utils.Errorf("invalid result: %s", lines[0])
	}
	var resources []*ypb.YakURLResource
	for _, line := range lines[2:] {

		// Skip empty lines.
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Split the line into fields.
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			return nil, fmt.Errorf("expected 5 fields, got %d: %s", len(fields), line)
		}
		// TODO CharsetDecode
		name := utils.EscapeInvalidUTF8Byte([]byte(fields[0]))
		typ := fields[1]
		lastModified := fields[2]
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid size: %s", fields[3])
		}
		perm := fields[4]

		newParam := &ypb.YakURL{
			Schema:   originParam.Schema,
			User:     originParam.GetUser(),
			Pass:     originParam.GetPass(),
			Location: originParam.GetLocation(),
			Query:    originParam.GetQuery(),
			Path:     filepath.Join(originParam.GetPath(), name),
		}

		var resource = &ypb.YakURLResource{
			Size:         size,
			SizeVerbose:  utils.ByteSize(uint64(size)),
			Path:         newParam.Path,
			Url:          newParam,
			ResourceName: name,
			VerboseName:  name,
			Extra:        []*ypb.KVPair{{Key: "perm", Value: perm}},
		}

		if typ == "0" {
			resource.ResourceType = "dir"
			resource.VerboseType = "godzilla-directory"
			resource.HaveChildrenNodes = true
		} else {
			resource.ResourceType = "file"
			resource.VerboseType = "godzilla-file"
			resource.HaveChildrenNodes = false
		}

		loc, _ := time.LoadLocation("Asia/Shanghai")

		// Parse the "lastModified" string to a Unix timestamp
		t, err := time.ParseInLocation("2006-01-02 15:04:05", lastModified, loc)
		if err == nil {
			resource.ModifiedTimestamp = t.Unix()
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

func parseBaseInfoToJson(data []byte) []byte {
	dataDict := make(map[string]interface{})
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	for _, r := range lines {
		if len(r) == 0 {
			continue
		}
		rL := strings.SplitN(r, " : ", 2)
		key := rL[0]
		value := ""
		if len(rL) == 2 {
			value = rL[1]
		}
		dataDict[key] = value
	}
	jsonStr := utils.Jsonify(dataDict)
	return jsonStr
}

func (g *GodzillaFileSystemAction) newGodzillaFormId(id string) (*Godzilla, error) {
	if g.godzillaCache == nil {
		g.godzillaCache = make(map[string]*Godzilla)
	}

	if manager, ok := g.godzillaCache[id]; ok {
		return manager, nil
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, utils.Errorf("cannot parse id[%s] as int: %s", id, err)
	}
	db := consts.GetGormProjectDatabase()
	shell, err := yakit.GetWebShell(db, int64(idInt))
	if err != nil {
		return nil, err
	}
	manager, err := NewGodzilla(shell)
	if err != nil {
		return nil, err
	}
	if shell.GetPacketCodecName() != "" {
		script, err := yakit.GetYakScriptByName(db, shell.GetPacketCodecName())
		if err != nil {
			return nil, err
		}

		manager.SetPacketScriptContent(script.Content)
	}
	if shell.GetPayloadCodecName() != "" {
		script, err := yakit.GetYakScriptByName(db, shell.GetPayloadCodecName())
		if err != nil {
			return nil, err
		}
		manager.SetPayloadScriptContent(script.Content)
	}
	g.godzillaCache[id] = manager
	return manager, nil
}

func (g *GodzillaFileSystemAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//u := params.GetUrl()
	//path := u.GetPath()
	//
	//var query = make(url.Values)
	//for _, v := range u.GetQuery() {
	//	query.Add(v.GetKey(), v.GetValue())
	//}
	//id := query.Get("id")
	//manager, err := g.newGodzillaFormId(id)
	//if err != nil {
	//	return nil, err
	//}
	//var res []*ypb.YakURLResource
	//switch query.Get("mode") {
	//case "list":
	//	//TODO implement me
	//	list, err := manager.getFile(path)
	//	if err != nil {
	//		return nil, err
	//	}
	//	res, err = godzillaResultToYakURLResource(u, list)
	//	if err != nil {
	//		return nil, err
	//	}
	//case "show":
	//
	//case "check":
	//case "checkExist":
	//
	//case "getTimeStamp":
	//
	//}

	//return &ypb.RequestYakURLResponse{
	//	Page:      1,
	//	PageSize:  100,
	//	Total:     int64(len(res)),
	//	Resources: res,
	//}, nil

	u := params.GetUrl()
	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	if query.Get("id") == "" {
		return nil, utils.Error("webshell id cannot be empty")
	}
	switch query.Get("op") {
	case "cmd":
		fallthrough
	case "db":
		return g.Do(params)
	case "file":
		//path := u.Path
		//if path == "" || path == "/" {
		path := query.Get("path")
		//}
		id := query.Get("id")
		manager, err := g.newGodzillaFormId(id)
		if err != nil {
			return nil, err
		}
		var res []*ypb.YakURLResource
		mode := query.Get("mode")

		funcMap := map[string]func() ([]byte, error){
			"list": func() ([]byte, error) {
				return manager.getFile(path)
			},
			//"show": func() ([]byte, error) {
			//	return manager.showFile(path)
			//},
			//"check": func() ([]byte, error) {
			//	return manager.checkFileHash(path, "")
			//},
			//"checkExist": func() ([]byte, error) {
			//	return manager.checkFileExist(path)
			//},
			//"getTimeStamp": func() ([]byte, error) {
			//	return manager.getTimeStamp(path)
			//},
		}

		// Call the function based on the mode
		if fn, ok := funcMap[mode]; ok {
			list, err := fn()
			if err != nil {
				return nil, err
			}
			res, err = godzillaResultToYakURLResource(u, list)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, utils.Errorf("unsupported mode %s", mode)
		}

		return &ypb.RequestYakURLResponse{
			Page:      1,
			PageSize:  100,
			Total:     int64(len(res)),
			Resources: res,
		}, nil
	default:
		return nil, utils.Errorf("unsupported op %s", query.Get("op"))

	}
}

func (g *GodzillaFileSystemAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GodzillaFileSystemAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GodzillaFileSystemAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GodzillaFileSystemAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GodzillaFileSystemAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()

	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	var res []*ypb.YakURLResource
	id := query.Get("id")
	manager, err := g.newGodzillaFormId(id)
	if err != nil {
		return nil, err
	}
	switch query.Get("op") {
	case "cmd":
		command := query.Get("cmd")
		path := query.Get("path")
		var resource = &ypb.YakURLResource{}
		if strings.HasPrefix(command, "cd ") {
			path, err = calculateNewPath(path, strings.TrimPrefix(command, "cd "))
			if err != nil {
				return nil, err
			}
			extra := []*ypb.KVPair{
				{Key: "content", Value: ""},
			}
			resource.Path = path
			resource.Extra = extra
		} else {
			// Todo 特征还是比较明显的
			fullCommand := "cd " + path + " && " + command
			raw, err := manager.CommandExec(fullCommand)
			if err != nil {
				return nil, err
			}
			content := gjson.GetBytes(raw, "msg").String()

			extra := []*ypb.KVPair{
				{Key: "content", Value: content},
			}
			resource.Path = path
			resource.Extra = extra
		}
		res = append(res, resource)

	case "db":

	default:
		return nil, utils.Errorf("unsupported op %s", query.Get("op"))
	}

	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (g *Godzilla) getFile(filePath string) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}
	parameter := newParameter()
	if len(filePath) == 0 {
		filePath = " "
	}
	parameter.AddString("dirName", filePath)
	res, err := g.EvalFunc("", "getFile", parameter)
	if err != nil {
		return nil, err
	}

	return res, nil
}

//func (g *Godzilla) downloadFile(fileName string) ([]byte, error) {
//	parameter := newParameter()
//
//	parameter.AddString("fileName", fileName)
//	result, err := g.EvalFunc("", "readFile", parameter)
//	if err != nil {
//		return nil, err
//	}
//
//	return result, nil
//}
//
//func (g *Godzilla) uploadFile(fileName string, data []byte) (bool, error) {
//	parameter := newParameter()
//
//	parameter.AddString("fileName", fileName)
//
//	parameter.AddBytes("fileValue", data)
//	result, err := g.EvalFunc("", "uploadFile", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(result)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}
//
//func (g *Godzilla) copyFile(fileName, newFile string) (bool, error) {
//	parameter := newParameter()
//	enfileName, err := g.encoding.CharsetEncode(fileName)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("srcFileName", enfileName)
//	enNewFile, err := g.encoding.CharsetEncode(newFile)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("destFileName", enNewFile)
//	result, err := g.EvalFunc("", "copyFile", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(result)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}
//
//func (g *Godzilla) deleteFile(fileName string) (bool, error) {
//	parameter := newParameter()
//	enfileName, err := g.encoding.CharsetEncode(fileName)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("fileName", enfileName)
//	result, err := g.EvalFunc("", "deleteFile", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(result)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}
//
//func (g *Godzilla) newFile(fileName string) (bool, error) {
//	parameter := newParameter()
//	enfileName, err := g.encoding.CharsetEncode(fileName)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("fileName", enfileName)
//	result, err := g.EvalFunc("", "newFile", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(result)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}
//
//func (g *Godzilla) moveFile(fileName, newFile string) (bool, error) {
//	parameter := newParameter()
//	enfileName, err := g.encoding.CharsetEncode(fileName)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("srcFileName", enfileName)
//	enNewFile, err := g.encoding.CharsetEncode(newFile)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("destFileName", enNewFile)
//	result, err := g.EvalFunc("", "moveFile", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(result)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}
//
//func (g *Godzilla) newDir(fileName string) (bool, error) {
//	parameter := newParameter()
//	enfileName, err := g.encoding.CharsetEncode(fileName)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("dirName", enfileName)
//	result, err := g.EvalFunc("", "newDir", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(result)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}
//
//func (g *Godzilla) bigFileUpload(fileName string, position int, content []byte) (string, error) {
//	parameter := newParameter()
//	enContent, err := g.encoding.CharsetEncode(string(content))
//	if err != nil {
//		return "", err
//	}
//	parameter.AddBytes("fileContents", enContent)
//	parameter.AddString("fileName", fileName)
//	parameter.AddString("position", strconv.Itoa(position))
//	result, err := g.EvalFunc("", "bigFileUpload", parameter)
//	if err != nil {
//		return "", err
//	}
//	decode, err := g.encoding.CharsetDecode(result)
//	if err != nil {
//		return "", err
//	}
//	return decode, nil
//}
//
//func (g *Godzilla) bigFileDownload(fileName string, position, readByteNum int) ([]byte, error) {
//	parameter := newParameter()
//	parameter.AddString("position", strconv.Itoa(position))
//	parameter.AddString("readByteNum", strconv.Itoa(readByteNum))
//	parameter.AddString("fileName", fileName)
//	parameter.AddString("mode", "read")
//	res, err := g.EvalFunc("", "bigFileDownload", parameter)
//	if err != nil {
//		return nil, err
//	}
//	return res, nil
//}
//func (g *Godzilla) fileRemoteDown(url, saveFile string) (bool, error) {
//	parameter := newParameter()
//	enUrl, err := g.encoding.CharsetEncode(url)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("url", enUrl)
//	enSaveFile, err := g.encoding.CharsetEncode(saveFile)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("saveFile", enSaveFile)
//	res, err := g.EvalFunc("", "fileRemoteDown", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(res)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}
//
//func (g *Godzilla) getFileSize(fileName string) (int, error) {
//	parameter := newParameter()
//	parameter.AddString("fileName", fileName)
//	parameter.AddString("mode", "fileSize")
//	result, err := g.EvalFunc("", "bigFileDownload", parameter)
//	if err != nil {
//		return -1, err
//	}
//	ret, err := strconv.Atoi(string(result))
//	if err != nil {
//		return -1, err
//	} else {
//		return ret, nil
//	}
//}
//
//func (g *Godzilla) setFileAttr(file, fileType, fileAttr string) (bool, error) {
//	parameter := newParameter()
//	parameter.AddString("type", fileType)
//	enfileName, err := g.encoding.CharsetEncode(file)
//	if err != nil {
//		return false, err
//	}
//	parameter.AddBytes("fileName", enfileName)
//	parameter.AddString("attr", fileAttr)
//	res, err := g.EvalFunc("", "setFileAttr", parameter)
//	if err != nil {
//		return false, err
//	}
//	decode, err := g.encoding.CharsetDecode(res)
//	if err != nil {
//		return false, err
//	}
//	if "ok" == decode {
//		return true, nil
//	} else {
//		return false, errors.New(decode)
//	}
//}

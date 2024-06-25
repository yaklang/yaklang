package yakurl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type fileSystemAction struct{}

func fileInfoToResource(originParam *ypb.YakURL, info os.FileInfo, currentPath string, inDir bool) *ypb.YakURLResource {
	newParam := &ypb.YakURL{
		Schema:   originParam.Schema,
		User:     originParam.GetUser(),
		Pass:     originParam.GetPass(),
		Location: originParam.GetLocation(),
		Path:     currentPath,
		Query:    originParam.GetQuery(),
	}
	if !inDir {
		newParam.Path = originParam.GetPath()
	}

	src := &ypb.YakURLResource{
		Size:              info.Size(),
		SizeVerbose:       utils.ByteSize(uint64(info.Size())),
		ModifiedTimestamp: info.ModTime().Unix(),
		Path:              currentPath,
		YakURLVerbose:     "",
		Url:               newParam,
	}
	if info.IsDir() {
		src.ResourceType = "dir"
		src.VerboseType = "filesystem-directory"
		infos, err := os.ReadDir(currentPath)
		if err == nil {
			src.HaveChildrenNodes = len(infos) > 0
		}
	}

	dirName, fileName := filepath.Split(currentPath)
	src.ResourceName = fileName
	if !info.IsDir() && info.Size() > 0 {
		src.VerboseName = fmt.Sprintf("%v [%v]", fileName, src.SizeVerbose)
	} else {
		src.VerboseName = fileName
	}
	src.Extra = append(src.Extra, &ypb.KVPair{
		Key:   "Directory-Name",
		Value: utils.EscapeInvalidUTF8Byte([]byte(dirName)),
	})
	return src
}

func (f fileSystemAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	absPath, _, _, err := FormatPath(params)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, utils.Errorf("cannot stat path[%s]: %s", u.GetPath(), err)
	}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	var res []*ypb.YakURLResource
	switch query.Get("op") {
	case "list":
		if info.IsDir() {
			infos, err := os.ReadDir(absPath)
			if err != nil {
				return nil, utils.Errorf("cannot read dir[%s]: %s", u.GetPath(), err)
			}
			for _, i := range infos {
				info, _ := i.Info()
				if info == nil {
					continue
				}
				currentPath := filepath.Join(params.GetUrl().Path, info.Name())
				res = append(res, fileInfoToResource(params.GetUrl(), info, currentPath, true))
			}
		}
	default:
		res = append(res, fileInfoToResource(params.GetUrl(), info, absPath, false))
	}
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (f fileSystemAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	absPath, dirname, _, err := FormatPath(params)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, utils.Errorf("cannot stat path[%s]: %s", u.GetPath(), err)
	}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	switch query.Get("op") {
	case "rename":
		newName := query.Get("newname")
		if newName == "" {
			return nil, utils.Errorf("newname is required")
		}

		if !filepath.IsAbs(newName) { // Compatible abs path and relative path
			newName = filepath.Join(dirname, newName)
		}

		err := os.Rename(absPath, newName)
		if err != nil {
			return nil, utils.Errorf("cannot rename file[%s]: %s", u.GetPath(), err)
		}

		absPath = newName
	case "content":
		fallthrough
	default:
		if info.IsDir() {
			return nil, utils.Errorf("cannot post to a directory: %s", u.GetPath())
		}
		err = os.WriteFile(absPath, params.GetBody(), 0644)
		if err != nil {
			return nil, utils.Errorf("cannot write file[%s]: %s", u.GetPath(), err)
		}
	}

	if YakRunnerMonitor != nil && utils.IsSubPath(absPath, YakRunnerMonitor.WatchPatch) {
		err = YakRunnerMonitor.UpdateFileTree()
		if err != nil {
			log.Errorf("failed to update file tree: %s", err)
		}
	}

	currentInfo, err := os.Stat(absPath)
	if err != nil {
		return nil, utils.Errorf("cannot stat path[%s]: %s", u.GetPath(), err)
	}
	res := fileInfoToResource(params.GetUrl(), currentInfo, absPath, false)
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     1,
		Resources: []*ypb.YakURLResource{res},
	}, nil
}

func (f fileSystemAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	absPath, _, _, err := FormatPath(params)
	if err != nil {
		return nil, err
	}
	exists, err := utils.PathExists(absPath)
	if exists {
		return nil, utils.Errorf("file [%s] exists", u.GetPath()) //  if file exists can't use put
	}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	switch query.Get("type") {
	case "dir":
		err := os.MkdirAll(absPath, 0755)
		if err != nil {
			return nil, err
		}
	case "file":
		fallthrough
	default:
		fileCreate, err := os.Create(absPath)
		if err != nil {
			return nil, err
		}
		defer fileCreate.Close()
		_, err = fileCreate.Write(params.GetBody())
		if err != nil {
			return nil, utils.Errorf("cannot write file[%s]: %s", u.GetPath(), err)
		}
	}

	if YakRunnerMonitor != nil && utils.IsSubPath(absPath, YakRunnerMonitor.WatchPatch) {
		err = YakRunnerMonitor.UpdateFileTree()
		if err != nil {
			log.Errorf("failed to update file tree: %s", err)
		}
	}

	currentInfo, err := os.Stat(absPath)
	if err != nil {
		return nil, utils.Errorf("cannot stat path[%s]: %s", u.GetPath(), err) // check file / dir
	}
	res := fileInfoToResource(params.GetUrl(), currentInfo, absPath, false)
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     1,
		Resources: []*ypb.YakURLResource{res},
	}, nil
}

func (f fileSystemAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	absPath, _, _, err := FormatPath(params)
	if err != nil {
		return nil, err
	}

	exists, err := utils.PathExists(absPath)
	if !exists {
		return nil, utils.Errorf("file [%s] exists check error: %s", u.GetPath(), err)
	}
	err = os.RemoveAll(absPath)
	if err != nil {
		return nil, err
	}
	if YakRunnerMonitor != nil && utils.IsSubPath(absPath, YakRunnerMonitor.WatchPatch) {
		err = YakRunnerMonitor.UpdateFileTree()
		if err != nil {
			log.Errorf("failed to update file tree: %s", err)
		}
	}
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     0,
		Resources: []*ypb.YakURLResource{},
	}, nil
}

func (f fileSystemAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fileSystemAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func FormatPath(params *ypb.RequestYakURLParams) (string, string, string, error) {
	u := params.GetUrl()
	var absPath string
	var dirname, filename string
	if filepath.IsAbs(u.GetPath()) {
		dirname, filename = filepath.Split(u.GetPath())
		absPath = u.GetPath()
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return "", "", "", utils.Errorf("cannot get current working directory: %s", err)
		}
		absPath = filepath.Join(wd, u.GetPath())
		dirname, filename = filepath.Split(absPath)
	}
	if params.GetUrl() != nil {
		params.Url.Path = absPath
	}
	return absPath, dirname, filename, nil
}

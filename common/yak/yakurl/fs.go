package yakurl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"os"
	"path/filepath"
)

type fileSystemAction struct {
}

func fileInfoToResource(originParam *ypb.YakURL, info os.FileInfo, inDir bool) *ypb.YakURLResource {
	var newParam = &ypb.YakURL{
		Schema:   originParam.Schema,
		User:     originParam.GetUser(),
		Pass:     originParam.GetPass(),
		Location: originParam.GetLocation(),
		Path:     filepath.Join(originParam.GetPath(), info.Name()),
		Query:    originParam.GetQuery(),
	}
	if !inDir {
		newParam.Path = originParam.GetPath()
	}

	var src = &ypb.YakURLResource{
		Size:              info.Size(),
		SizeVerbose:       utils.ByteSize(uint64(info.Size())),
		ModifiedTimestamp: info.ModTime().Unix(),
		Path:              newParam.Path,
		YakURLVerbose:     "",
		Url:               newParam,
	}
	if info.IsDir() {
		src.ResourceType = "dir"
		src.VerboseType = "filesystem-directory"
		src.HaveChildrenNodes = true
	}

	dirName, fileName := filepath.Split(src.Path)
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
	var absPath string
	var dirname, filename string
	if filepath.IsAbs(u.GetPath()) {
		dirname, filename = filepath.Split(u.GetPath())
		absPath = u.GetPath()
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return nil, utils.Errorf("cannot get current working directory: %s", err)
		}
		absPath = filepath.Join(wd, u.GetPath())
		dirname, filename = filepath.Split(absPath)
	}
	if params.GetUrl() != nil {
		params.Url.Path = absPath
	}
	_ = filename
	_ = dirname

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, utils.Errorf("cannot stat path[%s]: %s", u.GetPath(), err)
	}

	var query = make(url.Values)
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
				if info != nil {
					res = append(res, fileInfoToResource(params.GetUrl(), info, true))
				}
			}
			goto END
		}
	}
	res = append(res, fileInfoToResource(params.GetUrl(), info, false))

END:
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (f fileSystemAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (f fileSystemAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fileSystemAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fileSystemAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fileSystemAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

package wsm

import (
	"encoding/json"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"path/filepath"
	"strconv"
)

type YakShellResourceAction struct {
	cache map[string]*YakShell
}

func (y *YakShellResourceAction) newYakShellById(_id string) (*YakShell, error) {
	id, err := strconv.Atoi(_id)
	if err != nil {
		return nil, err
	}
	if y.cache[_id] != nil {
		return y.cache[_id], nil
	}
	db := consts.GetGormProjectDatabase()
	shell, err := yakit.GetWebShell(db, int64(id))
	if err != nil {
		return nil, err
	}
	yakShell, err := NewYakShell(shell)
	if err != nil {
		return nil, err
	}
	y.cache[_id] = yakShell
	return yakShell, nil
}
func (y *YakShellResourceAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return y.Do(params)
}

func (y *YakShellResourceAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return y.Do(params)
}

func (y *YakShellResourceAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return y.Do(params)
}

func (y *YakShellResourceAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return y.Do(params)
}

func (y *YakShellResourceAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (y *YakShellResourceAction) buildFileResource(originParam *ypb.YakURL, mode string, result []byte) ([]*ypb.YakURLResource, error) {
	type ResourceError struct {
		resources []*ypb.YakURLResource
		err       error
	}
	resErr := &ResourceError{}
	gjson.GetBytes(result, "msg").ForEach(func(_, v gjson.Result) bool {
		newParam := &ypb.YakURL{
			Schema:   originParam.Schema,
			User:     originParam.GetUser(),
			Pass:     originParam.GetPass(),
			Location: originParam.GetLocation(),
		}
		query := originParam.GetQuery()
		for _, v := range query {
			if v.GetKey() == "path" {
				newParam.Query = append(newParam.Query, &ypb.KVPair{
					Key:   v.GetKey(),
					Value: originParam.GetPath(),
				})
			} else {
				newParam.Query = append(newParam.Query, &ypb.KVPair{
					Key:   v.GetKey(),
					Value: v.GetValue(),
				})
			}
		}
		if mode == payloads.DirInfo.String() {
			var fileInfo []payloads.FileBaseInfo
			err := json.Unmarshal(result, &fileInfo)
			if err != nil {
				resErr.err = err
				return true
			}
			for _, info := range fileInfo {
				newParam.Path = filepath.Join(originParam.GetPath(), info.Filename)
				var resource = &ypb.YakURLResource{
					Size:              int64(info.GetSize()),
					SizeVerbose:       utils.ByteSize(uint64(info.GetSize())),
					Path:              newParam.Path,
					Url:               newParam,
					ResourceName:      info.Filename,
					VerboseName:       info.Filename,
					ResourceType:      info.Type,
					VerboseType:       info.Type,
					ModifiedTimestamp: info.GetTime(),
					HaveChildrenNodes: info.HasChildNodes(),
					Extra: []*ypb.KVPair{{
						Key:   "permission",
						Value: info.Permission,
					}},
				}
				resErr.resources = append(resErr.resources, resource)
			}
		}
		if mode == payloads.DbOperation.String() {
			//todo
		}
		if v.Type == gjson.String {
			name := filepath.Base(originParam.GetPath())
			newParam.Path = originParam.GetPath()
			var resource = &ypb.YakURLResource{
				Path:         newParam.Path,
				Url:          newParam,
				ResourceName: name,
				VerboseName:  name,
				Extra: []*ypb.KVPair{{
					Key:   "msg",
					Value: v.String(),
				}},
			}
			resErr.resources = append(resErr.resources, resource)
		}
		return true
	})
	return resErr.resources, resErr.err
}
func (y *YakShellResourceAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	var query = make(map[string]string, 2)
	for _, v := range u.GetQuery() {
		query[v.GetKey()] = v.GetValue()
	}
	var mode = query["mode"]
	var res []*ypb.YakURLResource
	_ = res
	id := query["id"]
	yakshell, err := y.newYakShellById(id)
	if err != nil {
		return nil, err
	}
	var result []byte
	_ = result
	var _err error
	switch query["mode"] {
	case payloads.CmdGo.String():
		result, _err = yakshell.CommandExec(query["command"])
	case payloads.BasicInfoGo.String():
		result, _err = yakshell.BasicInfo()
	default:
		delete(query, "id")
		result, _err = yakshell.ExecutePluginOrCache(query)
	}
	if _err != nil {
		return nil, _err
	}
	resource, err := y.buildFileResource(u, mode, result)
	if err != nil {
		return nil, err
	}
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(resource)),
		Resources: res,
	}, nil
}

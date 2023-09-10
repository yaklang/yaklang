package wsm

import (
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"path/filepath"
	"strconv"
	"time"
)

type BehidnerFileSystemAction struct {
}

func webShellResultToYakURLResource(originParam *ypb.YakURL, result []byte) ([]*ypb.YakURLResource, error) {
	var resources []*ypb.YakURLResource

	gjson.GetBytes(result, "msg").ForEach(func(_, v gjson.Result) bool {
		name := v.Get("name").String()
		size := v.Get("size").Int()
		typ := v.Get("type").String()
		lastModified := v.Get("lastModified").String()
		perm := v.Get("perm").String()

		newParam := &ypb.YakURL{
			Schema:   originParam.Schema,
			User:     originParam.GetUser(),
			Pass:     originParam.GetPass(),
			Location: originParam.GetLocation(),
			Path:     filepath.Join(originParam.GetPath(), name),
			Query:    originParam.GetQuery(), // 增加这一行来复制查询参数
		}

		var resource = &ypb.YakURLResource{
			Size:         size,
			SizeVerbose:  utils.ByteSize(uint64(size)),
			Path:         newParam.Path,
			Url:          newParam,
			ResourceName: name,
			VerboseName:  name,
			Extra: []*ypb.KVPair{
				{Key: "perm", Value: perm},
			},
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

		resources = append(resources, resource)
		return true // keep iterating
	})

	return resources, nil
}
func (b BehidnerFileSystemAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	path := u.GetPath()

	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	id := query.Get("id")
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
	var res []*ypb.YakURLResource
	switch query.Get("mode") {
	case "list":
		//TODO implement me
		list, err := manager.listFile(path)
		if err != nil {
			return nil, err
		}
		res, err = webShellResultToYakURLResource(u, list)
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

func (b BehidnerFileSystemAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (b BehidnerFileSystemAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (b BehidnerFileSystemAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (b BehidnerFileSystemAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (b BehidnerFileSystemAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

type ListFiles struct{}

func (l *ListFiles) Execute(base BaseShellManager) ([]byte, error) {
	// code to list files
	//base.
	return nil, nil
}

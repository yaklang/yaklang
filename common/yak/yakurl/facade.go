package yakurl

import (
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type facadeServerAction struct {
	BaseAction
}

var _ Action = (*facadeServerAction)(nil)

func newFacadeServerAction() *facadeServerAction {
	f := &facadeServerAction{}
	f.handle("GET", "resource", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		token, err := getParam("token")
		if err != nil {
			return nil, err
		}
		server := facades.GetFacadeServer(token)
		if server == nil {
			return nil, utils.Errorf("not found facade server by token: %s", token)
		}
		var yakResources []*ypb.YakURLResource
		resources := server.GetAllResourcesInfo()
		for _, resource := range resources {
			yakResources = append(yakResources, &ypb.YakURLResource{
				Extra: []*ypb.KVPair{
					{
						Key:   "Protocol",
						Value: resource.Protocol,
					},
					{
						Key:   "url",
						Value: resource.Url,
					},
					{
						Key:   "DataVerbose",
						Value: resource.DataVerbose,
					},
				},
			})
		}
		return &ypb.RequestYakURLResponse{
			Resources: yakResources,
		}, nil
	})
	f.handle("PUT", "resource", func(paramGetter func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		protocol, err := paramGetter("protocol")
		if err != nil {
			return nil, err
		}
		token, err := paramGetter("token")
		if err != nil {
			return nil, err
		}
		name, err := paramGetter("name")
		if err != nil {
			return nil, err
		}
		id, err := paramGetter("id")
		if err != nil {
			return nil, err
		}

		server := facades.GetFacadeServer(token)
		if server == nil {
			return nil, utils.Errorf("not found facade server by token: %s", token)
		}
		err = server.SetResource(protocol, name, id, body)
		if err != nil {
			return nil, err
		}
		return &ypb.RequestYakURLResponse{}, err
	})
	return f
}

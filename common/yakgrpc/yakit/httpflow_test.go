package yakit

import (
	"fmt"
	"github.com/go-rod/rod/lib/utils"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

//func TestYieldHTTPUrl(t *testing.T) {
//	forest := assets.NewWebsiteForest(10000)
//
//	db := consts.GetGormProjectDatabase()
//	db = db.Where("url LIKE '%baidu.com%'").Limit(10)
//	res := YieldHTTPUrl(
//		db, context.Background())
//	count := 0
//	for r := range res {
//		count++
//		println(r.Url)
//		forest.AddNode(r.Url)
//		if count > 10 {
//			break
//		}
//	}
//	raw, err := json.Marshal(forest.Output())
//	if err != nil {
//		return
//	}
//	println(string(raw))
//}
//
//func TestDeleteHTTPFlow(t *testing.T) {
//	DeleteHTTPFlow(consts.GetGormProjectDatabase().Debug(), &ypb.DeleteHTTPFlowRequest{URLPrefix: "https://github.com"})
//}
//
//func TestConvertFuzzerResponse(t *testing.T) {
//	FuzzerResponseToHTTPFlow(nil, &ypb.FuzzerResponse{
//		RequestRaw: []byte(`POST / HTTP/1.1
//Content-Type: application/json
//Host: www.example.com
//
//{"key": "value"}`),
//	})
//}

func TestHTTPFlow_Inset_FixUrl(t *testing.T) {
	token := utils.RandString(10)
	httpsFlow := &HTTPFlow{
		Url: fmt.Sprintf("https://baidu.com:443?a=%s", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), httpsFlow)

	httpFlow := &HTTPFlow{
		Url: fmt.Sprintf("http://baidu.com:80?a=%s", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), httpFlow)

	_, flows, err := QueryHTTPFlow(consts.GetGormProjectDatabase().Debug(), &ypb.QueryHTTPFlowRequest{Keyword: token})
	if err != nil {
		t.Fatal(err)
	}
	for _, flow := range flows {
		if flow.ID == httpsFlow.ID {
			if flow.Url != "https://baidu.com?a="+token {
				t.Fatal("insert fix https url error")
			}
			CreateOrUpdateHTTPFlow(consts.GetGormProjectDatabase().Debug(), flow.Hash, &HTTPFlow{
				Url: fmt.Sprintf("https://baidu.com:443?a=%s", token),
			})
		}

		if flow.ID == httpFlow.ID {
			if flow.Url != "http://baidu.com?a="+token {
				t.Fatal("insert fix http url error")
			}
			CreateOrUpdateHTTPFlow(consts.GetGormProjectDatabase().Debug(), flow.Hash, &HTTPFlow{
				Url: fmt.Sprintf("http://baidu.com:80?a=%s", token),
			})
		}
	}

	for _, flow := range flows {
		if flow.ID == httpsFlow.ID {
			if flow.Url != "https://baidu.com?a="+token {
				t.Fatal("update fix https url error")
			}
		}

		if flow.ID == httpFlow.ID {
			if flow.Url != "http://baidu.com?a="+token {
				t.Fatal("update fix http url error")
			}
		}
	}

}

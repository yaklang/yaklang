package yakit

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

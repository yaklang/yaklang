package lowhttp

//func TestNewWebHookServer(t *testing.T) {
//	NewWebHookServerEx(22234, func(data interface{}) {
//		spew.Dump(data)
//	}).Start()
//
//	time.Sleep(1200 * time.Millisecond)
//	client := &http.Client{
//		Transport: &http.Transport{
//			MaxIdleConns:        1,
//			MaxIdleConnsPerHost: 1,
//			MaxConnsPerHost:     1,
//			IdleConnTimeout:     30 * time.Second,
//		},
//	}
//	for range make([]int, 1000) {
//		go func() {
//			rsp, err := client.Get("http://127.0.0.1:22234/webhook")
//			if err != nil {
//				log.Error(err)
//				return
//			}
//			_ = rsp
//		}()
//	}
//	time.Sleep(10 * time.Minute)
//
//}

package yakgrpc

import (
	"testing"
)

func TestMUSTPASS_MITM_CONFIG(t *testing.T) {
	//database := consts.GetGormProjectDatabase().Debug()
	//yakit.DeleteHTTPFlow(database, &ypb.DeleteHTTPFlowRequest{DeleteAll: true})
	//consts.SetGlobalMaxContentLength(1024)
	//defer consts.SetGlobalMaxContentLength(1024 * 1024 * 10)
	//client, err := NewLocalClient()
	//require.NoError(t, err)
	//ctx, cancelFunc := context.WithTimeout(context.Background(), time.Duration(10)*time.Second)
	//defer cancelFunc()
	//stream, err := client.MITM(ctx)
	//require.NoError(t, err)
	//mitmPort := utils.GetRandomAvailableTCPPort()
	//data := bytes.Repeat([]byte("a"), 1024*2)
	//address, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
	//	return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %v\r\n\r\n%s", len(data), data))
	//})
	//stream.Send(&ypb.MITMRequest{
	//	Forward:        true,
	//	SetAutoForward: true,
	//	Host:           "127.0.0.1",
	//	Port:           uint32(mitmPort),
	//})
	//
	//for {
	//	rsp, err := stream.Recv()
	//	if err != nil {
	//		log.Errorf("mitm recv error: %s", err)
	//		continue
	//	}
	//	if rsp.GetMessage().GetIsMessage() && strings.Contains(string(rsp.GetMessage().GetMessage()), `starting mitm server`) {
	//		log.Infof("mitm success start")
	//		break
	//	}
	//}
	//addressx := fmt.Sprintf("http://%s:%v", address, port)
	//poc.DoGET(addressx, poc.WithProxy(fmt.Sprintf("http://127.0.0.1:%v", mitmPort)), poc.WithTimeout(1024))
	//time.Sleep(time.Second)
	//var flag bool
	//pading, httpflows, err := yakit.QueryHTTPFlow(database, &ypb.QueryHTTPFlowRequest{
	//	SearchURL: addressx,
	//	Full:      true,
	//})
	//for _, httpflow := range httpflows {
	//	marshal, err := json.Marshal(httpflow)
	//	require.NoError(t, err)
	//	fmt.Println("flowData: :", string(marshal))
	//	if httpflow.IsTooLargeResponse {
	//		flag = true
	//	}
	//}
	//require.NoError(t, err)
	//require.True(t, pading.TotalRecord != 0)
	//require.True(t, flag)
}

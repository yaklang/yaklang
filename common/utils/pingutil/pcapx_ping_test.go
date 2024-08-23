package pingutil

//func TestPcapxPing(t *testing.T) {
//	targets := utils.ParseStringToHosts(`47.52.100.1/20`)
//	swg := utils.NewSizedWaitGroup(30)
//	for _, ip := range targets {
//		ip := ip
//		swg.Add()
//		go func() {
//			defer swg.Done()
//			result, err := PcapxPing(ip, NewPingConfig())
//
//			if err != nil {
//				t.Fatal(err)
//			}
//			if result.Ok {
//				fmt.Println(result.IP + " is alive")
//			}
//		}()
//	}
//	swg.Wait()
//}

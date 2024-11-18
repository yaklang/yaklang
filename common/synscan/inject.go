package synscan

import (
	"github.com/gopacket/gopacket"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

func (s *Scanner) sleepRateLimit() {
	if s == nil {
		return
	}
	if s.delayMs <= 0 {
		return
	}
	time.Sleep(time.Duration(s.delayMs*1000) * time.Microsecond)
}

//func (s *Scanner) sendService() {
//	var counter int
//	var total int64
//	for {
//		if s.delayMs > 0 && s.delayGapCount > 0 {
//			if counter > s.delayGapCount {
//				counter = 0
//				//fmt.Printf("rate limit trigger! for %vms\n", s.delayMs)
//				s.sleepRateLimit()
//			}
//		}
//		select {
//		case localPackets, ok := <-s.localHandlerWriteChan:
//			if !ok {
//				continue
//			}
//
//			err := s.localHandler.WritePacketData(localPackets)
//
//			total++
//			counter++
//
//			if err != nil {
//				//log.Errorf("loopback handler write failed: %s", err)
//			}
//		case packets, ok := <-s.handlerWriteChan:
//			if !ok {
//				continue
//			}
//
//			failedCount := 0
//		RETRY_WRITE_IF:
//			// 5-15 us (每秒可以开到 1000 * 200 个包最快)
//			err := s.handler.WritePacketData(packets)
//
//			total++
//			counter++
//
//			if err != nil {
//				switch true {
//				case utils.IContains(err.Error(), "no buffer space available"):
//					if failedCount > 10 {
//						log.Errorf("write device failed: for %v", err.Error())
//						break
//					}
//					if s.delayMs > 0 {
//						s.sleepRateLimit()
//					} else {
//						time.Sleep(time.Millisecond * 10)
//					}
//					failedCount++
//					goto RETRY_WRITE_IF
//				default:
//					log.Errorf("iface: %v handler write failed: %s: retry", s.iface, err)
//				}
//			}
//		case <-s.ctx.Done():
//			return
//		}
//	}
//}

func (s *Scanner) inject(loopback bool, l ...gopacket.SerializableLayer) error {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("serialize layer or send to chan failed: %s", err)
		}
	}()

	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, s.opts, l...); err != nil {
		return errors.Errorf("serialize failed: %s", err)
	}
	ret := buf.Bytes()

	if !loopback && s.handlerIsAlive.IsSet() {
		s.handlerWriteChan <- ret
	} else if loopback && s.localHandlerIsAlive.IsSet() {
		s.localHandlerWriteChan <- ret
	} else {
		return utils.Error("no handler available")
	}

	return nil
}

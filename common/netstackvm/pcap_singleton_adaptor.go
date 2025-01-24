package netstackvm

import (
	"sync"

	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
)

type pcapSingletonAdaptor struct {
	handle *pcap.Handle
}

var singletonAdaptor *pcapSingletonAdaptor

func GetSingletonAdaptor() *pcapSingletonAdaptor {
	if singletonAdaptor == nil {
		singletonAdaptor = &pcapSingletonAdaptor{}
		initSingletonAdaptor(singletonAdaptor)
	}
	return singletonAdaptor
}

var initSingletonAdaptorOnce sync.Once

func initSingletonAdaptor(s *pcapSingletonAdaptor) error {
	var err error
	initSingletonAdaptorOnce.Do(func() {
		s.handle, err = pcap.OpenLive("any", 1600, true, pcap.BlockForever)
		if err != nil {
			log.Errorf("failed to open pcap handle: %s", err)
		}
	})
	return err
}

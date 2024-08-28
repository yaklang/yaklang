package pcaputil

import "github.com/yaklang/pcap"

type PcapHandleOperation interface {
	SetBPFFilter(filter string) error
	Close()
	Stats() (stat *pcap.Stats, err error)
}

type MockPcapOperation struct {
}

func (m *MockPcapOperation) SetBPFFilter(filter string) error {
	return nil
}

func (m *MockPcapOperation) Close() {
}

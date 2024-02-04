package pcaputil

type PcapHandleOperation interface {
	SetBPFFilter(filter string) error
	Close()
}

type MockPcapOperation struct {
}

func (m *MockPcapOperation) SetBPFFilter(filter string) error {
	return nil
}

func (m *MockPcapOperation) Close() {
}

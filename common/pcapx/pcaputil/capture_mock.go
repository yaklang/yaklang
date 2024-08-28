package pcaputil

type PcapHandleOperation interface {
	SetBPFFilter(filter string) error
	close()
}

type MockPcapOperation struct {
}

func (m *MockPcapOperation) SetBPFFilter(filter string) error {
	return nil
}

func (m *MockPcapOperation) close() {
}

package pcaputil

import "testing"

func TestStart(t *testing.T) {
	err := Start(
		WithDebug(false),
		WithDevice("WLAN"),
		WithOutput("./output.pcap"),
	)
	if err != nil {
		t.Error(err)
	}
}

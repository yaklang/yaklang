package pcaputil

import "testing"

func TestStart(t *testing.T) {
	err := Start(
		WithDebug(false),
		WithDevice("WLAN"),
		WithSuricataFilter("./suricata-test.rules"),
		WithOutput("./output.pcap"),
	)
	if err != nil {
		t.Error(err)
	}
}

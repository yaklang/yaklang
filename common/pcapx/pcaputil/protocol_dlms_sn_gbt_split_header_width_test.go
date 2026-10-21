package pcaputil

import "testing"

func TestDLMSHDLCSNGBTSplitHeaderWidthNative(t *testing.T) {
	c, m := hdlcSNGBTSplitHeaderLoad(t, "width-controls.json", "ad420927623df5353fc6044a96f4caa91a3113561d0da608e2404451eab267ab", 8)
	hdlcSNGBTInformationNative(t, c, m)
}

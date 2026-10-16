package pcaputil

import "testing"

// Independent full-row carrier-primed baseline uses only the existing public session API.
func TestDLMSHDLCSNWriteExistingAPI(t *testing.T) {
	hdlcSNReadNative(t, hdlcSNWriteCases(t, "baseline-controls.json", "f82e19d6127c4edaf399c93b517212af3a9a8fa6045e2370f0fab0e996c1f487", 3), 41000, 4059)
}

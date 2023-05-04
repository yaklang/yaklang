package crep

import "testing"

func TestTransparentHijackManager_Hijacked(t *testing.T) {
	NewMITMServer(MITM_SetTransparentHijackMode(true))
}

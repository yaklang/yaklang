package crep

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

// MITM_UseEMudhra opts into the compromised eMudhra CA for TLS trust testing.
// It is disabled by default and does not replace the user's saved MITM CA.
// Clients with updated revocation information or CT enforcement will reject it.
func MITM_UseEMudhra(enabled bool) MITMConfig {
	return func(server *MITMServer) error {
		if !enabled {
			return nil
		}
		ca, key := tlsutils.GetEMudhraCAAndKey()
		if err := MITM_SetCaCertAndPrivKey(ca, key, server.gmCACert, server.gmCAKey)(server); err != nil {
			return err
		}
		log.Warn("MITM is using the compromised eMudhra CA for TLS trust testing; patched clients will reject it")
		return nil
	}
}

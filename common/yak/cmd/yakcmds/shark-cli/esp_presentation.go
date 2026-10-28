package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// This is a clear-header observation, not an SA, authentication, decryption or
// replay result. The full native contract must survive the bounded IPC snapshot;
// a label, service port or two integers alone cannot establish this identity.
func validESPHeaderObservation(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "esp" || e.Transport != "network" || e.Profile != "esp-observed-header" || e.Admission != "observed-IP-next-header" || e.Status != "limited" || e.Completeness != "headers" || e.Error != "" || e.ExpertCode != "" || len(e.Raw) < 8 {
		return false
	}
	if e.Session["Header Complete"] != true || e.Session["Algorithm Context Available"] != false || e.Session["Authentication Verified"] != false || e.Session["Extended Sequence Reconstructed"] != false || e.Session["Observation Scope"] != "clear ESP header and opaque remainder; no SA, algorithm, peer authentication or replay verdict" {
		return false
	}
	spi, spiOK := gpeHeaderInteger(e.Session["SPI"], 0xffffffff)
	sequence, sequenceOK := gpeHeaderInteger(e.Session["Sequence"], 0xffffffff)
	body, bodyOK := e.Session["Opaque Body"].([]byte)
	if !spiOK || spi == 0 || !sequenceOK || !bodyOK || spi != uint64(binary.BigEndian.Uint32(e.Raw[:4])) || sequence != uint64(binary.BigEndian.Uint32(e.Raw[4:8])) || !bytes.Equal(body, e.Raw[8:]) {
		return false
	}
	return e.SourceBytes.SHA256 == fmt.Sprintf("%x", sha256.Sum256(e.Raw))
}

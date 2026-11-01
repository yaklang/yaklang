package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// This is a clear-header observation, not an SA, authentication, decryption or
// replay result. The full native contract must survive the bounded IPC snapshot;
// a label, service port or two integers alone cannot establish this identity.
func validESPHeaderObservation(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "esp" || e.Status != "limited" || e.Completeness != "headers" || e.Error != "" || e.ExpertCode != "" || len(e.Raw) < 8 {
		return false
	}
	direct := e.Transport == "network" && e.Profile == "esp-observed-header" && e.Admission == "observed-IP-next-header"
	if !direct && !validESPNATTCarrier(e) {
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

func validESPNATTCarrier(e *pcaputil.ProtocolEvent) bool {
	if e.Transport != "udp" || e.Profile != "esp-natt-observed-header" || e.Rule != "ipsec_esp" || e.Entry != "ESPHeaderFields" || len(e.Raw) < 12 || len(e.Raw) > 65527 {
		return false
	}
	port := func(endpoint string) (uint64, bool) {
		host, service, err := net.SplitHostPort(endpoint)
		if err != nil || net.ParseIP(host) == nil {
			return 0, false
		}
		p, err := strconv.ParseUint(service, 10, 16)
		return p, err == nil && p != 0
	}
	src, srcOK := port(e.Source)
	dst, dstOK := port(e.Destination)
	return srcOK && dstOK && (e.Admission == "explicit-decode-as" || e.Admission == "observed-UDP4500-nonzero-SPI" && (src == 4500 || dst == 4500))
}

func validESPIncompleteObservation(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "esp" || e.Transport != "network" || e.Profile != "esp-observed-header" || e.Admission != "observed-IP-next-header" || e.Status != "incomplete" || e.Completeness != "incomplete" || e.ExpertCode != "NeedMore" || e.Error != "NeedMore: ESP capture is incomplete; only captured clear-header facts are available" {
		return false
	}
	if e.Session["Algorithm Context Available"] != false || e.Session["Authentication Verified"] != false || e.Session["Extended Sequence Reconstructed"] != false || e.Session["Observation Scope"] != "clear ESP header and opaque remainder; no SA, algorithm, peer authentication or replay verdict" {
		return false
	}
	if len(e.Raw) < 8 {
		if e.Session["Header Complete"] != false {
			return false
		}
		if _, present := e.Session["Sequence"]; present {
			return false
		}
		if _, present := e.Session["Opaque Body"]; present {
			return false
		}
		if len(e.Raw) < 4 {
			if _, present := e.Session["SPI"]; present {
				return false
			}
		} else if spi, ok := gpeHeaderInteger(e.Session["SPI"], 0xffffffff); !ok || spi != uint64(binary.BigEndian.Uint32(e.Raw[:4])) {
			return false
		}
	} else {
		// A complete clear header does not complete the declared carrier. The
		// native NeedMore reason retains that limit without inferring its cause.
		if e.Session["Header Complete"] != true {
			return false
		}
		spi, spiOK := gpeHeaderInteger(e.Session["SPI"], 0xffffffff)
		sequence, sequenceOK := gpeHeaderInteger(e.Session["Sequence"], 0xffffffff)
		body, bodyOK := e.Session["Opaque Body"].([]byte)
		if !spiOK || spi == 0 || !sequenceOK || !bodyOK || spi != uint64(binary.BigEndian.Uint32(e.Raw[:4])) || sequence != uint64(binary.BigEndian.Uint32(e.Raw[4:8])) || !bytes.Equal(body, e.Raw[8:]) {
			return false
		}
	}
	if len(e.Raw) == 0 {
		return e.SourceBytes.SHA256 == "" // finalizeEvidence does not hash absent bytes.
	}
	return e.SourceBytes.SHA256 == fmt.Sprintf("%x", sha256.Sum256(e.Raw))
}

func espIncompleteDecoderCovered(p gopacket.Packet, presentation packetPresentation, why string) bool {
	if !presentation.nativeESPIncomplete || presentation.protocol != "ESP" || presentation.status != "incomplete" || presentation.nativeESPBytes >= 8 || why != fmt.Sprintf("ESP clear header requires 8 bytes, got %d", presentation.nativeESPBytes) {
		return false
	}
	layer, ok := p.Layer(layers.LayerTypeIPSecESP).(*layers.IPSecESP)
	return ok && len(layer.LayerContents()) == presentation.nativeESPBytes && sha256.Sum256(layer.LayerContents()) == presentation.nativeESPSHA
}

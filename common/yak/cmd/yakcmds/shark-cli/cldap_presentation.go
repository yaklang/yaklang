package sharkcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Reuse the canonical envelope parser once before publishing a presentation.
// A port hint or edited worker metadata cannot authorize response-body fields.
func cldapResponseEnvelopeSummary(e *pcaputil.ProtocolEvent) string {
	if e.Protocol != "cldap" || e.Profile != "cldap-native" || e.Transport != "udp" ||
		e.Status != "context-required" || e.Completeness != "context-required" ||
		e.ExpertCode != string(pcaputil.ErrUnsupportedFeature) || len(e.Raw) == 0 || len(e.Raw) > 65535 ||
		e.FlowID != 0 || e.TCPFlowID != 0 || e.ResponseTo != 0 || e.TransactionID != 0 {
		return ""
	}
	fields, err := pcaputil.DecodeCLDAPDatagram(e.Raw, 4096)
	var typed *pcaputil.ProtocolError
	if !errors.As(err, &typed) || typed.Kind != pcaputil.ErrUnsupportedFeature ||
		fields["Response Body Opaque"] != true || fields["Response Decoded"] != false {
		return ""
	}
	expected, wantErr := json.Marshal(fields)
	observed, gotErr := json.Marshal(e.Session)
	if wantErr != nil || gotErr != nil || !bytes.Equal(expected, observed) {
		return ""
	}
	messages := mapList(fields["Messages"])
	if len(messages) == 0 {
		return ""
	}
	var names []string
	for _, message := range messages {
		if operation := number(message["Operation"]); operation != 0x64 && operation != 0x65 {
			return ""
		}
		names = append(names, textValue(message["Message Name"]))
	}
	return fmt.Sprintf("CLDAP %s · %d message envelope(s) · response body unparsed", strings.Join(names, " + "), len(messages))
}

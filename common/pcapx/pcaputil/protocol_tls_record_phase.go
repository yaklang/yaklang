package pcaputil

// A passive monitor cannot infer that the client received a captured SH.
// The sender's own final SH or successfully authenticated protected record
// does prove its local write protection state (RFC8446 5.2 and 6).
func (t *binTLS) tls13SenderNeedsRecordProtection(dir int) bool {
	return t.version == 0x304 && (t.protectedRecordObserved[dir] || t.finalHello && t.serverHelloDirectionKnown && int(t.serverHelloDirection) == dir)
}

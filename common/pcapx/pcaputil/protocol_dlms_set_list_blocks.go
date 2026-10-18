package pcaputil

import (
	"encoding/hex"
	"errors"
)

// Selected complete HDLC SET list service blocks. Shared next3/ACK2 carry no
// descriptor count and need a previously observed list transfer for this label.
func dlmsSetListBlockAPDU(p []byte) bool {
	return len(p) > 1 && (p[0] == 0xc1 && p[1] == 5 || p[0] == 0xc5 && p[1] == 4)
}
func (t *dlmsSetTransfer) profile() string {
	if t.initial.choice == 5 {
		return "dlms-hdlc-set-list-block"
	}
	return "dlms-hdlc-set-block"
}
func (t *dlmsSetTransfer) closeName() string {
	if t.initial.choice == 5 {
		return "Set-with-list-and-data-block"
	}
	return "Set-with-data-block"
}
func (f *binFlow) dlmsSetListBlockContext(w []byte, dir int) bool {
	p := dlmsAPDU(w)
	if dlmsSetListBlockAPDU(p) {
		return true
	}
	if f.dlms == nil || f.dlms.setTransfer == nil || f.dlms.setTransfer.initial.choice != 5 || len(p) < 3 || !(p[0] == 0xc1 && p[1] == 3 || p[0] == 0xc5 && p[1] == 2) {
		return false
	}
	old := f.dlms.setTransfer.initial
	if p[2] != old.flags {
		return false
	}
	at := 3
	end := len(w) - 3
	dst, _, err := dlmsAddress(w[:end], &at)
	if err != nil {
		return false
	}
	src, _, err := dlmsAddress(w[:end], &at)
	if err != nil {
		return false
	}
	if p[0] == 0xc1 {
		return dir == old.dir && src == old.source && dst == old.destination
	}
	return dir != old.dir && src == old.destination && dst == old.source
}
func (f *binFlow) finishDLMSSetListBlockData(t *dlmsSetTransfer, b *wrapperMessage, joined []byte, count int) error {
	c := wrapperListCursor{wire: joined, nodes: t.parameterNodes, maxDepth: f.a.budget.MaxRecursionDepth}
	n, enc, err := c.count()
	if err == nil && n > uint64(min(f.a.budget.MaxCollectionElements, wrapperListItems)) {
		err = dlmsError(ErrResourceExceeded, "assembled SET list value count exceeds budget")
	}
	if err == nil && n != uint64(t.initial.count) {
		err = dlmsError(ErrUnsupportedFeature, "SET descriptor/value count outside selected equal-count profile")
	}
	var values []map[string]any
	if err == nil {
		values = make([]map[string]any, 0, int(n))
		for i := 0; i < int(n); i++ {
			start := c.at
			var data map[string]any
			data, err = c.data(f.a.budget.MaxCollectionElements, 0)
			if err != nil {
				break
			}
			values = append(values, map[string]any{"index": i, "encoded_offset": start, "data": data, "raw_hex": hex.EncodeToString(joined[start:c.at]), "encoded_length": c.at - start})
		}
	}
	if err == nil && c.at != len(joined) {
		err = dlmsError(ErrMalformedMessage, "assembled SET value list has trailing bytes")
	}
	if err != nil {
		f.dlms.invalidate()
		var pe *ProtocolError
		if errors.As(err, &pe) {
			err = dlmsError(pe.Kind, "SET list data-block: "+pe.Message)
		}
		return err
	}
	b.fields["transfer_complete"], b.fields["assembled_values"], b.fields["assembled_value_count"], b.fields["assembled_value_count_encoding_hex"], b.fields["assembled_data_hex"], b.fields["observed_block_count"] = true, values, n, enc, hex.EncodeToString(joined), count
	return nil
}

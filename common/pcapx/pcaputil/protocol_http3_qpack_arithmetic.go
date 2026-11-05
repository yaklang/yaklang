package pcaputil

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// Keep the mathematical failure distinct from missing table state or an
// observer resource limit. Packet authentication does not make an overflowing
// absolute reference a valid lookup into a retained dynamic table.
type qpackArithmeticError struct {
	component   string
	left, right uint64
}

func (e *qpackArithmeticError) Error() string {
	return fmt.Sprintf("qpack: %s arithmetic overflows (%d + %d)", e.component, e.left, e.right)
}

func isQPACKArithmeticError(err error) bool {
	var e *qpackArithmeticError
	return errors.As(err, &e)
}

func (q *binQUIC) rejectH3QPACKArithmetic(st *quicStream, dir int, sid uint64, fr, info map[string]any, frames []map[string]any, headerBytes int, payload []byte, cause error) error {
	h3 := st.h3
	start := uint64(h3.parsed[dir])
	end := start + uint64(headerBytes+len(payload))
	fact := map[string]any{
		"Error Name": "QPACK_DECOMPRESSION_FAILED", "Scope": "field section could not be decoded; other streams remain independently inspectable",
		"Stream ID": sid, "Direction": uint64(dir), "Frame Offset": start, "Frame End Offset": end,
	}
	fact["Error"] = cause.Error()
	fact["Rejected QPACK Block"] = bytes.Clone(payload)
	fact["Byte Source"] = st.sources[dir].describe(sid, dir, start+uint64(headerBytes), end)
	// A previously recovered field can leave a historical blocked PDU on this
	// stream. Reuse it only when it contributes to this field's actual frame.
	// A later SETTINGS/encoder trigger is context and must not become its origin.
	frameSource := st.sources[dir].describe(sid, dir, start, end)
	parents, _ := frameSource["Parent PDUs"].([]uint64)
	var original uint64
	for _, parent := range parents {
		if original == 0 || parent == h3.blockedPDU[dir] {
			original = parent
		}
	}
	if original != 0 {
		fact["Original Field PDU ID"] = original
	}
	var arithmetic *qpackArithmeticError
	if errors.As(cause, &arithmetic) {
		// Decimal strings preserve exact operands through JSON/worker clients.
		// They describe derived arithmetic, not byte spans or decoded headers.
		fact["Arithmetic Witness"] = map[string]any{
			"Component": arithmetic.component, "Operator": "+",
			"Left Operand": strconv.FormatUint(arithmetic.left, 10), "Right Operand": strconv.FormatUint(arithmetic.right, 10),
			"Scope": "mathematical sum exceeds uint64; no wrapped table entry is selected",
		}
	}
	fr["QPACK Field Error"], info["QPACK Field Error"] = fact, fact
	h3.blocked[dir], h3.settingsWaiting[dir], h3.rememberedWaiting[dir] = false, false, false
	// Quarantine this field stream without rolling back earlier authenticated
	// encoder instructions, reusing the packet number, or retiring its owner.
	// Other independently parsed streams keep their own status and fields.
	return q.rejectH3Message(st, dir, sid, fr, info, frames, protocolError(ErrMalformedMessage, "HTTP3 QPACK_DECOMPRESSION_FAILED: %s", cause))
}

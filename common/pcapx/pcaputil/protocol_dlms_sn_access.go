package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// VariableAccessSpecification is shared by SN Read and Write. A selector is
// an observed octet, not proof that an object supports that selection. Its
// parameter consumes the same Data node/depth pool as all other parameters and
// the separate Write values; it must not start a fresh cursor or budget.
func wrapperSNAccess(c *wrapperListCursor, row map[string]any, choice byte, limit int) error {
	if choice != 2 && choice != 4 {
		return wrapperError(ErrUnsupportedFeature, "SN selected variable-name/parameterized-access choices2/4 only")
	}
	name, err := c.take(2)
	if err != nil {
		return err
	}
	// Preserve the complete name bit pattern, including high-bit names. No
	// resolved COSEM class/attribute or selector meaning is inferred.
	row["access_choice_raw"], row["access_choice"] = choice, "variable-name"
	row["variable_name"], row["variable_name_hex"] = binary.BigEndian.Uint16(name), hex.EncodeToString(name)
	if choice == 2 {
		return nil
	}
	selector, err := c.take(1)
	if err != nil {
		return err
	}
	parameter, err := c.data(limit, 0)
	if err != nil {
		return err
	}
	row["access_choice"] = "parameterized-access"
	row["selector"], row["parameter"] = selector[0], parameter
	row["selector_semantics_verified"] = false
	return nil
}

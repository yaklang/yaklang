package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func stpTestWire(version byte, pvst bool) []byte {
	w := []byte{0, 0, 0, 0, 0x3c, 0x80, 0x01, 2, 0, 0, 0, 0, 1, 0, 0, 0, 19, 0x90, 0x01, 2, 0, 0, 0, 0, 2, 0x80, 3, 0, 128, 20, 0, 2, 0, 15, 0}
	if version != 0 {
		w[2], w[3] = version, 2
		w = append(w, 0)
	}
	if version == 3 {
		w = append(w, 0, 80, 0)
		name := make([]byte, 32)
		copy(name, []byte("region-a"))
		w = append(w, name...)
		w = append(w, 0, 7)
		w = append(w, bytes.Repeat([]byte{0x12}, 16)...)
		w = append(w, 0, 0, 0, 11, 0x80, 0, 2, 0, 0, 0, 0, 3, 20)
		w = append(w, 0x7c, 0x80, 1, 2, 0, 0, 0, 0, 4, 0, 0, 0, 12, 0x90, 0x80, 19)
	}
	if pvst {
		if version == 0 {
			w = append(w, 0)
		}
		w = append(w, 0, 0, 0, 2, 0, 121)
	}
	return w
}

func TestSTPWireExactVariantsAndNonInheritedFields(t *testing.T) {
	for _, v := range []byte{0, 2, 3} {
		for _, pvst := range []bool{false, true} {
			if v == 3 && pvst {
				continue
			}
			w := stpTestWire(v, pvst)
			m, err := DecodeSTPWire(w, pvst, 128)
			require.NoError(t, err)
			require.EqualValues(t, v, m["Version"])
			require.EqualValues(t, 32768, m["Root Priority"])
			require.EqualValues(t, 0.5, m["Message Age Seconds"])
			require.EqualValues(t, 128, m["Message Age Ticks"])
			require.Equal(t, "02:00:00:00:00:01", m["Root MAC"])
			require.Equal(t, false, m["Authentication Verified"])
			for cut := 0; cut < len(w); cut++ {
				_, err = DecodeSTPWire(w[:cut], pvst, 128)
				require.Error(t, err, "v%d pvst%v cut%d", v, pvst, cut)
			}
			_, err = DecodeSTPWire(append(bytes.Clone(w), 0), pvst, 128)
			require.Error(t, err)
			_, err = DecodeSTPWire(append(bytes.Clone(w), w...), pvst, 128)
			require.Error(t, err)
			if v == 3 {
				rows := m["MST Instances"].([]map[string]any)
				require.Len(t, rows, 1)
				require.EqualValues(t, 1, rows[0]["Instance ID"])
				require.Equal(t, false, m["MST Digest Verified"])
			}
		}
	}
	for _, pvst := range []bool{false, true} {
		m, err := DecodeSTPWire([]byte{0, 0, 0, 128}, pvst, 4)
		require.NoError(t, err)
		require.NotContains(t, m, "Root MAC")
		require.NotContains(t, m, "Flags")
	}
	for _, change := range []func([]byte){func(w []byte) { w[0] = 1 }, func(w []byte) { w[2] = 1 }, func(w []byte) { w[3] = 0 }, func(w []byte) { w[35] = 1 }} {
		w := stpTestWire(2, false)
		change(w)
		_, err := DecodeSTPWire(w, false, 128)
		require.Error(t, err)
	}
}

func TestSTPMSTAndPVSTBoundaryAndCollectionRefusals(t *testing.T) {
	w := stpTestWire(3, false)
	for _, n := range []uint16{0, 1, 63, 64, 79, 81, 0xffff} {
		b := bytes.Clone(w)
		binary.BigEndian.PutUint16(b[36:38], n)
		_, err := DecodeSTPWire(b, false, 128)
		require.Error(t, err)
	}
	b := append(bytes.Clone(w), w[102:]...)
	binary.BigEndian.PutUint16(b[36:38], 96)
	_, err := DecodeSTPWire(b, false, 128)
	require.Error(t, err, "duplicate MSTI")
	b[120] = 2
	_, err = DecodeSTPWire(b, false, 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	w = stpTestWire(2, true)
	_, err = DecodeSTPWire(w, false, 128)
	require.Error(t, err, "PVST TLV cannot become an IEEE trailer")
	_, err = DecodeSTPWire(w[:36], true, 128)
	require.Error(t, err, "PVST carrier requires its originating VLAN")
	b = append(bytes.Clone(w), 0, 7, 0, 2, 0xff, 0xee)
	m, err := DecodeSTPWire(b, true, 128)
	require.NoError(t, err)
	require.Equal(t, true, m["Unselected Extensions"])
	_, err = DecodeSTPWire(b, true, 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	b = append(bytes.Clone(w), 0, 0, 0, 2, 0, 122)
	_, err = DecodeSTPWire(b, true, 128)
	require.Error(t, err, "conflicting originating VLAN")
}

func TestSTPConfigurationAndTCNCompatibilityKeepsLiteralVersion(t *testing.T) {
	for _, version := range []byte{0, 2, 3, 95} {
		w := stpTestWire(0, false)
		w[2] = version
		m, err := DecodeSTPWire(w, false, 64)
		require.NoError(t, err)
		require.EqualValues(t, version, m["Version"])
		require.Equal(t, "STP", m["Protocol Name"])
		require.NotContains(t, m, "Port Role")
		require.NotContains(t, m, "Version 1 Length")
		if version != 0 {
			require.NotEmpty(t, m["Conformance Warning"])
		}
		m, err = DecodeSTPWire([]byte{0, 0, version, 128}, false, 8)
		require.NoError(t, err)
		require.Equal(t, "STP", m["Protocol Name"])
		require.NotContains(t, m, "Root MAC")
	}
}

func FuzzSTPCompleteBoundedWire(f *testing.F) {
	for _, v := range []byte{0, 2, 3} {
		f.Add(stpTestWire(v, false), false)
		if v != 3 {
			f.Add(stpTestWire(v, true), true)
		}
	}
	f.Add([]byte{0, 0, 0, 128}, false)
	f.Fuzz(func(t *testing.T, w []byte, pvst bool) {
		if len(w) > 2048 {
			return
		}
		if m, err := DecodeSTPWire(w, pvst, 64); err == nil {
			require.EqualValues(t, len(w), m["PDU Length"])
			require.Equal(t, false, m["Authentication Verified"])
			require.NotEmpty(t, m["Protocol Name"])
		}
	})
}

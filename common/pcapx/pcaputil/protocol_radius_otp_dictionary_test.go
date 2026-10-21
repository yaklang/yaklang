package pcaputil

import (
	"encoding/hex"
	"testing"
)

func TestRADIUSOTPDictionaryIndependentVectors(t *testing.T) {
	t.Run("INCH SEA ANNE LONG AHEM TOUR", func(t *testing.T) {
		v, _, found, checksum := radiusOTPDecodeWords([]byte("INCH SEA ANNE LONG AHEM TOUR"))
		if !found || !checksum || hex.EncodeToString(v[:]) != "9e876134d90499dd" {
			t.Fatalf("fixedRFC2289 vector: found=%v checksum=%v got=%x", found, checksum, v)
		}
	})
	t.Run("EASE OIL FUM CURE AWRY AVIS", func(t *testing.T) {
		v, _, found, checksum := radiusOTPDecodeWords([]byte("EASE OIL FUM CURE AWRY AVIS"))
		if !found || !checksum || hex.EncodeToString(v[:]) != "7965e05436f5029f" {
			t.Fatalf("fixedRFC2289 vector: found=%v checksum=%v got=%x", found, checksum, v)
		}
	})
	t.Run("BAIL TUFT BITS GANG CHEF THY", func(t *testing.T) {
		v, _, found, checksum := radiusOTPDecodeWords([]byte("BAIL TUFT BITS GANG CHEF THY"))
		if !found || !checksum || hex.EncodeToString(v[:]) != "50fe1962c4965880" {
			t.Fatalf("fixedRFC2289 vector: found=%v checksum=%v got=%x", found, checksum, v)
		}
	})
	t.Run("FULL PEW DOWN ONCE MORT ARC", func(t *testing.T) {
		v, _, found, checksum := radiusOTPDecodeWords([]byte("FULL PEW DOWN ONCE MORT ARC"))
		if !found || !checksum || hex.EncodeToString(v[:]) != "87066dd9644bf206" {
			t.Fatalf("fixedRFC2289 vector: found=%v checksum=%v got=%x", found, checksum, v)
		}
	})
	t.Run("ABE ACE ADA ADD BAD A", func(t *testing.T) {
		v, _, found, checksum := radiusOTPDecodeWords([]byte("ABE ACE ADA ADD BAD A"))
		if !found || !checksum || hex.EncodeToString(v[:]) != "0020080280605600" {
			t.Fatalf("fixedRFC2289 vector: found=%v checksum=%v got=%x", found, checksum, v)
		}
	})
}
func TestRADIUSOTPStandardWordsKeepASCII(t *testing.T) {
	_, _, found, _ := radiusOTPDecodeWords([]byte("ınch SEA ANNE LONG AHEM TOUR"))
	if found {
		t.Fatal("Unicode case-fold cannot create a standard ASCII dictionary word")
	}
}

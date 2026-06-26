package javaclassparser

import "testing"

func TestDollarMethodIdentifierValidatorGap(t *testing.T) {
	header := "public class MAPX"
	member := "public MAPX $(java.lang.Object key, java.lang.Object value) {\n\treturn this;\n}"

	err := validateMemberInHeader(header, member)
	if err != nil && !isDollarIdentifierValidatorGap(member, err) {
		t.Fatalf("expected $ method declaration to be treated as validator gap, got: %v", err)
	}
}

package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCombinedNullDefaultLabel(t *testing.T) {
	_, err := Frontend(`class Example {
String order(String input) {
return switch (input) {
case "date" -> "folder.mod_date";
case null, default -> "folder.name";
};
}
}`)
	require.NoError(t, err)
}

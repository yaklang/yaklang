package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestForeachNilIterableDoesNotPanic guards the panic observed on grav's
// tests/conformance/run.php: a foreach whose iterable expression compiled to
// nil used to crash NewNext with nil pointer dereference.
func TestForeachNilIterableDoesNotPanic(t *testing.T) {
	src := `<?php
foreach ($missing as $v) {
    echo $v;
}
`
	prog, err := Parse(src, WithLanguage(ssaconfig.PHP))
	require.NoError(t, err)
	require.NotNil(t, prog)
}

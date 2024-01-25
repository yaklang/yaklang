package utils

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFaviconURL(t *testing.T) {
	var abc, err = ExtractFaviconURL("http://example.com/abc", []byte(`
<!DOCTYPE html>
<html>
<head>
	<link rel="shortcut icon" href="favicon.ico" type="image/x-icon">
	<link rel="icon" href="favicon.ico" type="image/x-icon">
	<link rel="icon" href="favicon.png" type="image/png">
	<link rel="icon" href="favicon.jpg" type="image/jpeg">
</head>
<body>
</body>
`))
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(abc)
	assert.Equal(t, "http://example.com/favicon.ico", abc)
}

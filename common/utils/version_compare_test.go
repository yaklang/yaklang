package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestVersionCompare(t *testing.T) {

	testcase := []struct {
		v1     string
		v2     string
		expect int
	}{
		{"1.1.1", "1.1.3", -1},
		{"15.3\\(3\\)m2", "15.3\\(3\\)m1", 1},
		{"15.3\\(3\\)m", "15.3\\(3\\)m1", -1},
		{"5.51beta", "5.51", -1},
		{"6.3.6rc1", "6.3.6rc2", -1},
		{"1.0alpha1", "1.0beta3", -1},
		{"11.50.xc5w2", "11.50.xc5w3", -1},
		{"2.13.4sp1", "2.13.4", 1},
		{"ozw772.04", "758-874", -2},
		{"6.x-2.0alpha8", "6.x-2.0beta", -1},
		{"1.5.0\\(222\\)", "1.5.0\\(2\\)", 1},
		{"5.3.2-1ubuntu4.17", "5.3.2-1ubuntu4.16", 1},
	}

	for _, i := range testcase {
		res, _ := VersionCompare(i.v1, i.v2)
		assert.Equal(t, i.expect, res, "")
	}
}

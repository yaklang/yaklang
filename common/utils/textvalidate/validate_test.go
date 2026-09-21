// Tests adapted from govalidator f21760c49a8d; see LICENSE.
package textvalidate

import "testing"

func TestIsInt(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		param    string
		expected bool
	}{
		{"-2147483648", true},          //Signed 32 Bit Min Int
		{"2147483647", true},           //Signed 32 Bit Max Int
		{"-2147483649", true},          //Signed 32 Bit Min Int - 1
		{"2147483648", true},           //Signed 32 Bit Max Int + 1
		{"4294967295", true},           //Unsigned 32 Bit Max Int
		{"4294967296", true},           //Unsigned 32 Bit Max Int + 1
		{"-9223372036854775808", true}, //Signed 64 Bit Min Int
		{"9223372036854775807", true},  //Signed 64 Bit Max Int
		{"-9223372036854775809", true}, //Signed 64 Bit Min Int - 1
		{"9223372036854775808", true},  //Signed 64 Bit Max Int + 1
		{"18446744073709551615", true}, //Unsigned 64 Bit Max Int
		{"18446744073709551616", true}, //Unsigned 64 Bit Max Int + 1
		{"", true},
		{"123", true},
		{"0", true},
		{"-0", true},
		{"+0", true},
		{"01", false},
		{"123.123", false},
		{" ", false},
		{"000", false},
	}
	for _, test := range tests {
		actual := IsInt(test.param)
		if actual != test.expected {
			t.Errorf("Expected IsInt(%q) to be %v, got %v", test.param, test.expected, actual)
		}
	}
}

func TestIsFloat(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		param    string
		expected bool
	}{
		{"", false},
		{"  ", false},
		{"-.123", false},
		{"abacaba", false},
		{"1f", false},
		{"-1f", false},
		{"+1f", false},
		{"123", true},
		{"123.", true},
		{"123.123", true},
		{"-123.123", true},
		{"+123.123", true},
		{"0.123", true},
		{"-0.123", true},
		{"+0.123", true},
		{".0", true},
		{"01.123", true},
		{"-0.22250738585072011e-307", true},
		{"+0.22250738585072011e-307", true},
	}
	for _, test := range tests {
		actual := IsFloat(test.param)
		if actual != test.expected {
			t.Errorf("Expected IsFloat(%q) to be %v, got %v", test.param, test.expected, actual)
		}
	}
}

func TestIsBase64(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		param    string
		expected bool
	}{
		{"TG9yZW0gaXBzdW0gZG9sb3Igc2l0IGFtZXQsIGNvbnNlY3RldHVyIGFkaXBpc2NpbmcgZWxpdC4=", true},
		{"Vml2YW11cyBmZXJtZW50dW0gc2VtcGVyIHBvcnRhLg==", true},
		{"U3VzcGVuZGlzc2UgbGVjdHVzIGxlbw==", true},
		{"MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAuMPNS1Ufof9EW/M98FNw" +
			"UAKrwflsqVxaxQjBQnHQmiI7Vac40t8x7pIb8gLGV6wL7sBTJiPovJ0V7y7oc0Ye" +
			"rhKh0Rm4skP2z/jHwwZICgGzBvA0rH8xlhUiTvcwDCJ0kc+fh35hNt8srZQM4619" +
			"FTgB66Xmp4EtVyhpQV+t02g6NzK72oZI0vnAvqhpkxLeLiMCyrI416wHm5Tkukhx" +
			"QmcL2a6hNOyu0ixX/x2kSFXApEnVrJ+/IxGyfyw8kf4N2IZpW5nEP847lpfj0SZZ" +
			"Fwrd1mnfnDbYohX2zRptLy2ZUn06Qo9pkG5ntvFEPo9bfZeULtjYzIl6K8gJ2uGZ" + "HQIDAQAB", true},
		{"12345", false},
		{"", false},
		{"Vml2YW11cyBmZXJtZtesting123", false},
	}
	for _, test := range tests {
		actual := IsBase64(test.param)
		if actual != test.expected {
			t.Errorf("Expected IsBase64(%q) to be %v, got %v", test.param, test.expected, actual)
		}
	}
}

func TestIsPrintableASCII(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		param    string
		expected bool
	}{
		{"", true},
		{"ｆｏｏbar", false},
		{"ｘｙｚ０９８", false},
		{"１２３456", false},
		{"ｶﾀｶﾅ", false},
		{"foobar", true},
		{"0987654321", true},
		{"test@example.com", true},
		{"1234abcDEF", true},
		{"newline\n", false},
		{"\x19test\x7F", false},
	}
	for _, test := range tests {
		actual := IsPrintableASCII(test.param)
		if actual != test.expected {
			t.Errorf("Expected IsPrintableASCII(%q) to be %v, got %v", test.param, test.expected, actual)
		}
	}
}

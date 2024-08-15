package regexp_utils

import "testing"

func TestYakRegexpManager(t *testing.T) {
	NewYakRegexpUtils("2[0-4]\\d(?#200-249)|25[0-5](?#250-255)|[01]?\\d\\d?(?#0-199)")
}

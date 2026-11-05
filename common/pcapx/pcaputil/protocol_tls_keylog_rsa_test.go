package pcaputil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSKeyLogLegacyRSAAndClientRandomAreDifferentNamespaces(t *testing.T) {
	random, master, premaster := bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 48), bytes.Repeat([]byte{0x33}, 48)
	rsa := fmt.Sprintf("RSA 0123456789abcdef %x\n", premaster)
	client := fmt.Sprintf("CLIENT_RANDOM %x %x\n", random, master)
	for _, log := range []string{rsa + client, client + rsa, rsa + rsa + client, "# both OpenSSL formats\r\n" + rsa + client} {
		p, err := ParseTLSKeyLog(log)
		require.NoError(t, err, "legacy premaster lines must not hide a usable master-secret line")
		require.Len(t, p.secrets, 1, "premaster must never be indexed as a ClientRandom secret")
		got, ok := p.LookupTLSSecret("CLIENT_RANDOM", random)
		require.True(t, ok)
		require.Equal(t, master, got)
		got[0] ^= 0xff
		again, ok := p.LookupTLSSecret("CLIENT_RANDOM", random)
		require.True(t, ok)
		require.Equal(t, master, again)
		_, ok = p.LookupTLSSecret("RSA", []byte{1, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef})
		require.False(t, ok)
	}
	p, err := ParseTLSKeyLog(rsa)
	require.NoError(t, err)
	require.Empty(t, p.secrets, "RSA-only logs do not authorize record decryption")
}

func TestTLSKeyLogAcceptsCRLFCRAndLFTerminators(t *testing.T) {
	for _, separator := range []string{"\r\n", "\r", "\n"} {
		t.Run(fmt.Sprintf("%x", separator), func(t *testing.T) {
			lines := []string{"# platform line endings", "", "RSA 0123456789abcdef " + strings.Repeat("33", 48),
				"CLIENT_RANDOM " + strings.Repeat("11", 32) + " " + strings.Repeat("22", 48), ""}
			p, err := ParseTLSKeyLog(strings.Join(lines, separator))
			require.NoError(t, err)
			value, ok := p.LookupTLSSecret("CLIENT_RANDOM", bytes.Repeat([]byte{0x11}, 32))
			require.True(t, ok)
			require.Equal(t, bytes.Repeat([]byte{0x22}, 48), value)
		})
	}
}

func TestTLSKeyLogLegacyRSARejectsMalformedAndConflictingEntries(t *testing.T) {
	valid := fmt.Sprintf("RSA 0123456789abcdef %x\n", bytes.Repeat([]byte{0x33}, 48))
	for _, input := range []string{
		"RSA 00 " + strings.Repeat("33", 48),
		"RSA " + strings.Repeat("11", 32) + " " + strings.Repeat("33", 48),
		"RSA 0123456789abcdeg " + strings.Repeat("33", 48),
		"RSA 0123456789abcdef " + strings.Repeat("33", 47),
		"RSA 0123456789abcdef " + strings.Repeat("33", 49),
		"RSA 0123456789abcdef " + strings.Repeat("gg", 48),
		valid + "RSA 0123456789abcdef " + strings.Repeat("44", 48),
	} {
		p, err := ParseTLSKeyLog(input)
		require.Error(t, err)
		require.Nil(t, p, "a conflicting key log cannot return a partially useful provider")
		require.NotContains(t, err.Error(), strings.Repeat("33", 8), "diagnostics cannot include key material")
	}
	_, err := ParseTLSKeyLog("CLIENT_RANDOM 00 " + strings.Repeat("33", 48))
	require.Error(t, err, "legacy support must not relax the 32-byte ClientRandom requirement")
}

func TestTLSKeyLogLegacyRSASharesEntryBudgetAndDuplicatesAreIdempotent(t *testing.T) {
	var log strings.Builder
	secret := strings.Repeat("33", 48)
	for i := 0; i < 4095; i++ {
		fmt.Fprintf(&log, "RSA %016x %s\n", i, secret)
	}
	random := strings.Repeat("11", 32)
	client := "CLIENT_RANDOM " + random + " " + strings.Repeat("22", 48) + "\n"
	log.WriteString(client)
	log.WriteString(client)
	fmt.Fprintf(&log, "RSA %016x %s\n", 0, secret)
	p, err := ParseTLSKeyLog(log.String())
	require.NoError(t, err, "duplicate entries do not consume additional budget")
	got, ok := p.LookupTLSSecret("CLIENT_RANDOM", bytes.Repeat([]byte{0x11}, 32))
	require.True(t, ok)
	require.Equal(t, strings.Repeat("22", 48), hex.EncodeToString(got))
	_, err = ParseTLSKeyLog(log.String() + fmt.Sprintf("RSA %016x %s\n", 4095, secret))
	require.ErrorContains(t, err, "budget")
	_, err = ParseTLSKeyLog(log.String() + "CLIENT_RANDOM " + strings.Repeat("44", 32) + " " + secret)
	require.ErrorContains(t, err, "budget")
}

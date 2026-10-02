package stream_parser

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	pcre2 "github.com/VillanCh/go-pcre2-lite"
	"github.com/stretchr/testify/require"
)

type xmlrpcDeclarationSample struct {
	name, input string
	want        bool
}

func xmlrpcDeclarationSamples(size int) []xmlrpcDeclarationSample {
	return []xmlrpcDeclarationSample{
		{"valid_minimal", `<?xml version="1.0"?>`, true},
		{"valid_full", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`, true},
		{"valid_single_quotes", `<?xml version='1.0' encoding='utf-8' standalone='no'?>`, true},
		{"valid_multiline", "<?xml\tversion\n=\r'1.0'\nencoding='uTf-8'\r\n?>", true},
		{"invalid_version", `<?xml version="1.1"?>`, false},
		{"invalid_missing_version", `<?xml encoding="UTF-8"?>`, false},
		{"invalid_duplicate", `<?xml version="1.0" version="1.0"?>`, false},
		{"invalid_encoding", `<?xml version="1.0" encoding="UTF-16"?>`, false},
		{"invalid_space", "<?xml\u00a0version='1.0'?>", false},
		{"invalid_closing", `<?xml version="1.0"? >`, false},
		{"long_valid", "<?xml" + strings.Repeat(" ", size) + `version="1.0"?>`, true},
		{"long_invalid", `<?xml version="1.0"` + strings.Repeat(" ", size) + `standalone="maybe"?>`, false},
	}
}

func TestXMLRPCDeclarationPrecompiledRE2Equivalence(t *testing.T) {
	// The source is the original RE2-compatible literal; RE2 remains only an
	// independent test oracle, not a second production matcher.
	oracle := regexp.MustCompile(xmlrpcDeclaration.String())
	inputs := append(xmlrpcDeclarationSamples(64<<10), xmlrpcDeclarationSamples((1<<20)-128)...)
	for _, sample := range inputs {
		require.LessOrEqual(t, len(sample.input), 1<<20)
		got, err := xmlrpcDeclaration.MatchString(sample.input)
		require.NoError(t, err, sample.name)
		require.Equal(t, sample.want, oracle.MatchString(sample.input), sample.name)
		require.Equal(t, sample.want, got, sample.name)
	}
	count := len(inputs)
	for _, sample := range xmlrpcDeclarationSamples(0)[:10] {
		for i := 0; i < len(sample.input); i++ {
			mutated := []byte(sample.input)
			mutated[i] = '#'
			for _, input := range []string{sample.input[:i], string(mutated)} {
				got, err := xmlrpcDeclaration.MatchString(input)
				require.NoError(t, err)
				require.Equal(t, oracle.MatchString(input), got, "%s mutation at %d", sample.name, i)
				count++
			}
		}
	}
	require.Equal(t, 694, count)
	for _, suffix := range []string{"\n", "\r\n", "\r", "tail", "\x00", "\u2028"} {
		input := `<?xml version="1.0"?>` + suffix
		got, err := xmlrpcDeclaration.MatchString(input)
		require.NoError(t, err)
		require.False(t, oracle.MatchString(input))
		require.False(t, got, "DollarEndOnly must reject suffix %q", suffix)
	}
}

func TestXMLRPCDeclarationMatchErrorsRemainDistinct(t *testing.T) {
	valid := `<?xml version="1.0"?>`
	require.NoError(t, validateXMLRPCDeclaration(xmlrpcDeclaration, valid))
	require.EqualError(t, validateXMLRPCDeclaration(xmlrpcDeclaration, "not a declaration"), "xmlrpc: invalid declaration or unsupported processing instruction")
	closed, err := pcre2.Compile(xmlrpcDeclaration.String(), pcre2.CompileOptions{DollarEndOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closed.Close()) })
	require.NoError(t, closed.Close())
	err = validateXMLRPCDeclaration(closed, valid)
	require.ErrorIs(t, err, pcre2.ErrClosed)
	require.ErrorContains(t, err, "xmlrpc: declaration match failed")
	limited, err := pcre2.Compile(xmlrpcDeclaration.String(), pcre2.CompileOptions{DollarEndOnly: true, MatchLimit: 1})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, limited.Close()) })
	err = validateXMLRPCDeclaration(limited, valid)
	require.ErrorIs(t, err, pcre2.ErrMatchLimit)
	require.ErrorContains(t, err, "xmlrpc: declaration match failed")
}

func TestXMLRPCDeclarationConcurrentSharedMatcher(t *testing.T) {
	const workers, repeats = 64, 64
	inputs := xmlrpcDeclarationSamples(1024)
	const body = `<methodCall><methodName>read</methodName></methodCall>`
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	start := make(chan struct{})
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for i := 0; i < repeats; i++ {
				sample := inputs[(worker+i)%len(inputs)]
				got, err := xmlrpcDeclaration.MatchString(sample.input)
				if err != nil || got != sample.want {
					errors <- fmt.Errorf("worker %d iteration %d: match=%v error=%v", worker, i, got, err)
					return
				}
				message, err := decodeXMLRPCText(sample.input+body, 64)
				if (err == nil) != sample.want || (err == nil && (message == nil || message.Method != "read")) {
					errors <- fmt.Errorf("worker %d iteration %d: decode=%v error=%v", worker, i, message, err)
					return
				}
			}
		}(worker)
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func TestXMLRPCDeclarationRetainsDocumentSizeAndUTF8Bounds(t *testing.T) {
	const body = `<methodCall><methodName>read</methodName></methodCall>`
	const fixed = `<?xml version="1.0"?>`
	spaces := (1 << 20) - len(fixed) - len(body)
	document := "<?xml" + strings.Repeat(" ", spaces+1) + `version="1.0"?>` + body
	require.Len(t, document, 1<<20)
	message, err := decodeXMLRPCText(document, 64)
	require.NoError(t, err)
	require.Equal(t, "read", message.Method)
	_, err = decodeXMLRPCText(document+" ", 64)
	require.EqualError(t, err, "xmlrpc: invalid text size, encoding or depth limit")
	_, err = decodeXMLRPCText(fixed+body+"\xff", 64)
	require.EqualError(t, err, "xmlrpc: invalid text size, encoding or depth limit")
	// The declaration's final newline belongs to document whitespace, not the
	// declaration slice passed to the matcher by encoding/xml.
	_, err = decodeXMLRPCText(fixed+"\n"+body, 64)
	require.NoError(t, err)
}

func FuzzXMLRPCDeclarationParity(f *testing.F) {
	oracle := regexp.MustCompile(xmlrpcDeclaration.String())
	for _, sample := range xmlrpcDeclarationSamples(0) {
		f.Add(sample.input)
	}
	f.Add("<?xml version='1.0'?>\n")
	f.Add("<?xml version='1.0'?>\x00")
	f.Add("<?xml\xffversion='1.0'?>")
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 64<<10 {
			t.Skip()
		}
		got, err := xmlrpcDeclaration.MatchString(input)
		require.NoError(t, err)
		require.Equal(t, oracle.MatchString(input), got)
	})
}

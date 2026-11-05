package pythonparser

import (
	"fmt"
	"regexp"
	"strings"
)

// UnsupportedPython2Error marks source the Python 3 grammar rejects because it
// uses Python 2 only syntax. The file cannot be compiled; callers skip it and
// report one diagnostic instead of treating the parse failure as an engine
// error. Err keeps the original parser error for the diagnostic detail.
type UnsupportedPython2Error struct {
	// Reason is the first Python 2 construct found, e.g. "except binding".
	Reason string
	Err    error
}

func (e *UnsupportedPython2Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("python 2 syntax is not supported (%s): %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("python 2 syntax is not supported (%s)", e.Reason)
}

func (e *UnsupportedPython2Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// UnsupportedSyntaxReason implements the ssareducer skip contract without
// importing that package: a file with a reason is skipped, not failed.
func (e *UnsupportedPython2Error) UnsupportedSyntaxReason() string {
	if e == nil {
		return ""
	}
	return "python 2 syntax: " + e.Reason
}

type python2Feature struct {
	reason string
	re     *regexp.Regexp
}

// The patterns target constructs Python 3 never accepts. They run only after
// the Python 3 parse already failed, so a false positive can only turn one
// syntax error into one skip, never hide a compiling file.
var python2Features = []python2Feature{
	{"except binding", regexp.MustCompile(`(?m)^[ \t]*except[ \t]+[^\n:]+,[ \t]*[A-Za-z_]\w*[ \t]*:`)},
	{"ur string prefix", regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])ur['"]`)},
	{"print statement", regexp.MustCompile(`(?m)^[ \t]*print[ \t]+(>>|[^(\n=])`)},
	{"exec statement", regexp.MustCompile(`(?m)^[ \t]*exec[ \t]+['"]`)},
	{"raise with comma", regexp.MustCompile(`(?m)^[ \t]*raise[ \t]+[^\n,]+,[ \t]*[^\n]+$`)},
	{"backtick repr", regexp.MustCompile("`[^`\n]+`")},
	{"long integer literal", regexp.MustCompile(`\b\d+[lL]\b`)},
	{"octal literal", regexp.MustCompile(`\b0[0-7]{2,}\b`)},
	{"<> operator", regexp.MustCompile(`[^<]<>[^>]`)},
	{"lambda tuple parameter", regexp.MustCompile(`lambda[ \t]*\([^)\n]*,[^)\n]*\)[ \t]*:`)},
	{"async module import", regexp.MustCompile(`(?m)^[ \t]*(?:import|from)[ \t]+[^\n#]*\basync\b`)},
}

// DetectUnsupportedPython2 classifies source after a failed Python 3 parse.
// It returns nil when no Python 2 only construct is present, which leaves the
// original syntax error to the caller.
func DetectUnsupportedPython2(src string) *UnsupportedPython2Error {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	for _, feature := range python2Features {
		if feature.re.MatchString(src) {
			return &UnsupportedPython2Error{Reason: feature.reason}
		}
	}
	return nil
}

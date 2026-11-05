package pythonparser

import (
	"fmt"
	"regexp"
)

// UnsupportedTemplateError marks a .py file that is actually a Jinja or Django
// template. Such files live in template directories (cookiecutter scaffolds,
// Django app templates) but carry a .py suffix. The Python parser cannot read
// them; callers skip them like Python 2 sources and report one diagnostic.
type UnsupportedTemplateError struct {
	// Reason is the first template construct found, e.g. "jinja block tag".
	Reason string
	Err    error
}

func (e *UnsupportedTemplateError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("template syntax is not python (%s): %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("template syntax is not python (%s)", e.Reason)
}

func (e *UnsupportedTemplateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// UnsupportedSyntaxReason implements the ssareducer skip contract without
// importing that package.
func (e *UnsupportedTemplateError) UnsupportedSyntaxReason() string {
	if e == nil {
		return ""
	}
	return "template syntax: " + e.Reason
}

type templateFeature struct {
	reason string
	re     *regexp.Regexp
}

// Match only at the start of a line. An inline "{{ value }}" inside a Python
// string is valid Python and parses before this check runs. A line that starts
// with a template tag cannot be Python.
var templateFeatures = []templateFeature{
	{"jinja block tag", regexp.MustCompile(`(?m)^[ \t]*\{%`)},
	{"jinja expression", regexp.MustCompile(`(?m)^[ \t]*\{\{`)},
	{"jinja comment", regexp.MustCompile(`(?m)^[ \t]*\{#`)},
	{"scaffold import", regexp.MustCompile(`(?m)^[ \t]*(?:from|import)[ \t]+[^\n]*\$\{`)},
	{"scaffold placeholder", regexp.MustCompile(`(?m)^[ \t]*\$\{`)},
}

// DetectUnsupportedTemplate classifies source after a failed Python 3 parse.
// It returns nil when no line starts with a template tag.
func DetectUnsupportedTemplate(src string) *UnsupportedTemplateError {
	for _, feature := range templateFeatures {
		if feature.re.MatchString(src) {
			return &UnsupportedTemplateError{Reason: feature.reason}
		}
	}
	return nil
}

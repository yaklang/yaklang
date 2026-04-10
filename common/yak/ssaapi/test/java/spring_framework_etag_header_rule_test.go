package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegexETagHeaderParsingAuditRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-400-uncontrolled-resource-consumption/java-regex-etag-header-parsing-audit.sf")
	code := `
class Matcher {
    boolean find() { return false; }
}

class Pattern {
    Matcher matcher(String value) { return null; }
}

class HttpHeaders {
    private static final Pattern ETAG_HEADER_VALUE_PATTERN = new Pattern();

    void getETagValuesAsList(String value) {
        Matcher matcher = ETAG_HEADER_VALUE_PATTERN.matcher(value);
        while (matcher.find()) {
        }
    }
}

class ServletWebRequest {
    private static final Pattern ETAG_HEADER_VALUE_PATTERN = new Pattern();

    void matchRequestedETags(String value) {
        Matcher etagMatcher = ETAG_HEADER_VALUE_PATTERN.matcher(value);
        while (etagMatcher.find()) {
        }
    }
}
`
	counts := runJavaRule(t, rule, "ETagParsing.java", code)
	assert.GreaterOrEqual(t, counts["risk"], 2)
}

func TestRegexETagHeaderParsingAuditRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-400-uncontrolled-resource-consumption/java-regex-etag-header-parsing-audit.sf")
	code := `
class ETag {
    static void parse(String value) {
    }
}

class HttpHeaders {
    void getETagValuesAsList(String value) {
        ETag.parse(value);
    }
}

class ServletWebRequest {
    void matchRequestedETags(String value) {
        ETag.parse(value);
    }
}
`
	counts := runJavaRule(t, rule, "ETagParsingSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

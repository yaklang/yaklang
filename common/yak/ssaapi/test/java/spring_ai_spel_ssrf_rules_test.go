package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterExpressionConvertedToSpELRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-94-code-injection/java-filter-expression-converted-to-spel.sf")
	code := `
class SearchRequest {
    Object getFilterExpression() { return null; }
}

class StandardEvaluationContext {
    void setVariable(String name, Object value) {
    }
}

class Parser {
    Parsed parseExpression(String expression) { return null; }
}

class Parsed {
    Boolean getValue(StandardEvaluationContext context, Class<Boolean> clazz) { return Boolean.TRUE; }
}

class Converter {
    String convertExpression(Object expression) { return ""; }
}

class SimpleVectorStore {
    private final Parser expressionParser = new Parser();
    private final Converter filterExpressionConverter = new Converter();

    boolean doFilterPredicate(SearchRequest request, Object metadata) {
        StandardEvaluationContext context = new StandardEvaluationContext();
        context.setVariable("metadata", metadata);
        return this.expressionParser
                .parseExpression(this.filterExpressionConverter.convertExpression(request.getFilterExpression()))
                .getValue(context, Boolean.class);
    }
}
`
	counts := runJavaRule(t, rule, "SimpleVectorStore.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestFilterExpressionConvertedToSpELRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-94-code-injection/java-filter-expression-converted-to-spel.sf")
	code := `
class SearchRequest {
    Object getFilterExpression() { return null; }
}

class Evaluator {
    boolean evaluate(Object expression, Object metadata) { return true; }
}

class SimpleVectorStore {
    private final Evaluator filterExpressionEvaluator = new Evaluator();

    boolean doFilterPredicate(SearchRequest request, Object metadata) {
        return this.filterExpressionEvaluator.evaluate(request.getFilterExpression(), metadata);
    }
}
`
	counts := runJavaRule(t, rule, "SimpleVectorStoreSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

func TestRemoteMediaFetchWithoutStrictURLValidationRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-918-ssrf/java-remote-media-fetch-without-strict-url-validation.sf")
	code := `
import java.io.InputStream;
import java.net.URL;

class Media {
    Object getData() { return null; }
}

class BedrockProxyChatModel {
    Object mapMediaToContentBlock(Media media) throws Exception {
        if (media.getData() instanceof String text) {
            URL url = new URL(text);
            try (InputStream is = url.openConnection().getInputStream()) {
                return is;
            }
        }
        else if (media.getData() instanceof URL url) {
            try (InputStream is = url.openConnection().getInputStream()) {
                return is;
            }
        }
        return null;
    }
}
`
	counts := runJavaRule(t, rule, "BedrockProxyChatModel.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestRemoteMediaFetchWithoutStrictURLValidationRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-918-ssrf/java-remote-media-fetch-without-strict-url-validation.sf")
	code := `
import java.net.URI;
import java.net.URL;

class Media {
    Object getData() { return null; }
}

class MediaFetcher {
    byte[] fetch(URI uri) { return null; }
}

class BedrockProxyChatModel {
    private final MediaFetcher mediaFetcher = new MediaFetcher();

    Object mapMediaToContentBlock(Media media) throws Exception {
        if (media.getData() instanceof String text) {
            return this.mediaFetcher.fetch(URI.create(text));
        }
        else if (media.getData() instanceof URL url) {
            return this.mediaFetcher.fetch(url.toURI());
        }
        return null;
    }
}
`
	counts := runJavaRule(t, rule, "BedrockProxyChatModelSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

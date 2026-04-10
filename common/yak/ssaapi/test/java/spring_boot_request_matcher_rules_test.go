package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActuatorAdditionalPathRequestMatcherWildcardRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-863-incorrect-authorization/java-actuator-additional-path-request-matcher-wildcard.sf")
	code := `
import java.util.Set;

class RequestMatcherFactory {
}

class RequestMatcherProvider {
}

class PathMappedEndpoints {
    Set<String> getAdditionalPaths(Object namespace, Object endpointId) { return null; }
}

class AdditionalPathsEndpointRequestMatcher {
    Object webServerNamespace;
    java.util.List<Object> endpoints;
    Object httpMethod;

    Object getDelegateMatchers(RequestMatcherFactory requestMatcherFactory, RequestMatcherProvider matcherProvider,
            Set<String> paths, Object httpMethod) {
        return null;
    }

    Object createDelegate(PathMappedEndpoints endpoints, RequestMatcherFactory requestMatcherFactory,
            RequestMatcherProvider matcherProvider) {
        Set<String> paths = this.endpoints.stream()
                .filter(java.util.Objects::nonNull)
                .flatMap((endpointId) -> endpoints.getAdditionalPaths(this.webServerNamespace, endpointId).stream())
                .collect(java.util.stream.Collectors.toCollection(java.util.LinkedHashSet::new));
        return getDelegateMatchers(requestMatcherFactory, matcherProvider, paths, this.httpMethod);
    }
}
`
	counts := runJavaRule(t, rule, "AdditionalPathsEndpointRequestMatcher.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestActuatorAdditionalPathRequestMatcherWildcardRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-863-incorrect-authorization/java-actuator-additional-path-request-matcher-wildcard.sf")
	code := `
class RequestMatcherFactory {
    Object antPath(Object matcherProvider, Object httpMethod, String path) { return null; }
}

class RequestMatcherProvider {
}

class PathMappedEndpoints {
    java.util.List<String> getAdditionalPaths(Object namespace, Object endpointId) { return null; }
}

class AdditionalPathsEndpointRequestMatcher {
    Object webServerNamespace;
    java.util.List<Object> endpoints;
    Object httpMethod;

    Object createDelegate(PathMappedEndpoints endpoints, RequestMatcherFactory requestMatcherFactory,
            RequestMatcherProvider matcherProvider) {
        java.util.List<Object> matchers = this.endpoints.stream()
                .filter(java.util.Objects::nonNull)
                .flatMap((endpointId) -> endpoints.getAdditionalPaths(this.webServerNamespace, endpointId).stream())
                .map((path) -> requestMatcherFactory.antPath(matcherProvider, this.httpMethod, path))
                .toList();
        return matchers;
    }
}
`
	counts := runJavaRule(t, rule, "AdditionalPathsEndpointRequestMatcherSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

func TestCloudFoundryBasePathMatcherWithoutCatchAllRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-863-incorrect-authorization/java-cloudfoundry-base-path-matcher-without-catchall.sf")
	code := `
class PathMappedEndpoints {
    void getAllPaths() {
    }
}

class Config {
    static final String BASE_PATH = "/cfApplication";

    Object pathMatcher(String path) { return null; }

    Object getRequestMatcher(PathMappedEndpoints endpoints) {
        endpoints.getAllPaths().forEach((path) -> pathMatcher(path + "/**"));
        pathMatcher(BASE_PATH);
        pathMatcher(BASE_PATH + "/");
        return null;
    }
}
`
	counts := runJavaRule(t, rule, "CloudFoundryMatcherConfig.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestCloudFoundryBasePathMatcherWithoutCatchAllRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-863-incorrect-authorization/java-cloudfoundry-base-path-matcher-without-catchall.sf")
	code := `
class Config {
    static final String BASE_PATH = "/cfApplication";

    Object pathMatcher(String path) { return null; }

    Object getRequestMatcher() {
        return pathMatcher(BASE_PATH + "/**");
    }
}
`
	counts := runJavaRule(t, rule, "CloudFoundryMatcherConfigSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

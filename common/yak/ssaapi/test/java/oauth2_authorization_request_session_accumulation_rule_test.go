package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadOAuth2AuthorizationRequestSessionAccumulationRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-400-uncontrolled-resource-consumption/java-oauth2-authorization-request-session-accumulation.sf")
	if !ok {
		t.Skip("java-oauth2-authorization-request-session-accumulation.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-oauth2-authorization-request-session-accumulation.sf 内容为空")
	return content
}

func TestOAuth2AuthorizationRequestSessionAccumulationRule_Positive_HttpSession(t *testing.T) {
	rule := loadOAuth2AuthorizationRequestSessionAccumulationRule(t)
	code := `
import java.util.HashMap;
import java.util.Map;

class OAuth2AuthorizationRequest {
    String getState() { return "state"; }
}

interface HttpSession {
    void setAttribute(String name, Object value);
}

interface HttpServletRequest {
    HttpSession getSession();
}

interface HttpServletResponse {
}

class AuthorizationRequestRepository {
    private final String sessionAttributeName = "authz";

    Map<String, OAuth2AuthorizationRequest> getAuthorizationRequests(HttpServletRequest request) {
        return new HashMap<>();
    }

    void saveAuthorizationRequest(OAuth2AuthorizationRequest authorizationRequest, HttpServletRequest request,
            HttpServletResponse response) {
        String state = authorizationRequest.getState();
        Map<String, OAuth2AuthorizationRequest> authorizationRequests = this.getAuthorizationRequests(request);
        authorizationRequests.put(state, authorizationRequest);
        request.getSession().setAttribute(this.sessionAttributeName, authorizationRequests);
    }
}
`

	counts := runJavaRule(t, rule, "AuthorizationRequestRepository.java", code)
	assert.Greater(t, counts["risk"], 0, "按 state 把授权请求 Map 写回 HttpSession 时应触发告警")
}

func TestOAuth2AuthorizationRequestSessionAccumulationRule_Positive_WebSession(t *testing.T) {
	rule := loadOAuth2AuthorizationRequestSessionAccumulationRule(t)
	code := `
import java.util.HashMap;
import java.util.Map;

class OAuth2AuthorizationRequest {
    String getState() { return "state"; }
}

class Mono<T> {
    Mono<T> doOnNext(java.util.function.Consumer<T> c) { return this; }
    Mono<Void> then() { return new Mono<Void>(); }
}

class ServerWebExchange {
}

class AuthorizationRequestRepository {
    private final String sessionAttributeName = "authz";

    Mono<Map<String, Object>> getSessionAttributes(ServerWebExchange exchange) {
        return new Mono<Map<String, Object>>();
    }

    Map<String, OAuth2AuthorizationRequest> getAuthorizationRequests(Map<String, Object> sessionAttrs) {
        return new HashMap<>();
    }

    Mono<Void> saveAuthorizationRequest(OAuth2AuthorizationRequest authorizationRequest, ServerWebExchange exchange) {
        return getSessionAttributes(exchange)
                .doOnNext((sessionAttrs) -> {
                    Map<String, OAuth2AuthorizationRequest> authorizationRequests = this.getAuthorizationRequests(sessionAttrs);
                    authorizationRequests.put(authorizationRequest.getState(), authorizationRequest);
                    sessionAttrs.put(this.sessionAttributeName, authorizationRequests);
                })
                .then();
    }
}
`

	counts := runJavaRule(t, rule, "ReactiveAuthorizationRequestRepository.java", code)
	assert.Greater(t, counts["risk"], 0, "按 state 把授权请求 Map 写回 WebSession attributes 时应触发告警")
}

func TestOAuth2AuthorizationRequestSessionAccumulationRule_Negative_SingleRequest(t *testing.T) {
	rule := loadOAuth2AuthorizationRequestSessionAccumulationRule(t)
	code := `
class OAuth2AuthorizationRequest {
}

interface HttpSession {
    void setAttribute(String name, Object value);
}

interface HttpServletRequest {
    HttpSession getSession();
}

interface HttpServletResponse {
}

class AuthorizationRequestRepository {
    private final String sessionAttributeName = "authz";

    void saveAuthorizationRequest(OAuth2AuthorizationRequest authorizationRequest, HttpServletRequest request,
            HttpServletResponse response) {
        request.getSession().setAttribute(this.sessionAttributeName, authorizationRequest);
    }
}
`

	counts := runJavaRule(t, rule, "AuthorizationRequestRepositorySafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "只保存单个授权请求对象时不应由该规则报出")
}

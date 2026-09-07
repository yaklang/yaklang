package buildin_rule

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type springRuleCase struct {
	name       string
	rulePath   string
	fileName   string
	code       string
	wantAlerts map[string]int
}

func TestJavaSpringRuleRegressionCases(t *testing.T) {
	cases := []springRuleCase{
		{
			name:       "actuator-additional-path-request-matcher-wildcard-negative",
			rulePath:   "java/cwe-863-incorrect-authorization/java-actuator-additional-path-request-matcher-wildcard.sf",
			fileName:   "AdditionalPathsEndpointRequestMatcherSafe.java",
			code:       "class RequestMatcherFactory {\n    Object antPath(Object matcherProvider, Object httpMethod, String path) { return null; }\n}\n\nclass RequestMatcherProvider {\n}\n\nclass PathMappedEndpoints {\n    java.util.List<String> getAdditionalPaths(Object namespace, Object endpointId) { return null; }\n}\n\nclass AdditionalPathsEndpointRequestMatcher {\n    Object webServerNamespace;\n    java.util.List<Object> endpoints;\n    Object httpMethod;\n\n    Object createDelegate(PathMappedEndpoints endpoints, RequestMatcherFactory requestMatcherFactory,\n            RequestMatcherProvider matcherProvider) {\n        java.util.List<Object> matchers = this.endpoints.stream()\n                .filter(java.util.Objects::nonNull)\n                .flatMap((endpointId) -> endpoints.getAdditionalPaths(this.webServerNamespace, endpointId).stream())\n                .map((path) -> requestMatcherFactory.antPath(matcherProvider, this.httpMethod, path))\n                .toList();\n        return matchers;\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "actuator-additional-path-request-matcher-wildcard-positive",
			rulePath:   "java/cwe-863-incorrect-authorization/java-actuator-additional-path-request-matcher-wildcard.sf",
			fileName:   "AdditionalPathsEndpointRequestMatcher.java",
			code:       "import java.util.Set;\n\nclass RequestMatcherFactory {\n}\n\nclass RequestMatcherProvider {\n}\n\nclass PathMappedEndpoints {\n    Set<String> getAdditionalPaths(Object namespace, Object endpointId) { return null; }\n}\n\nclass AdditionalPathsEndpointRequestMatcher {\n    Object webServerNamespace;\n    java.util.List<Object> endpoints;\n    Object httpMethod;\n\n    Object getDelegateMatchers(RequestMatcherFactory requestMatcherFactory, RequestMatcherProvider matcherProvider,\n            Set<String> paths, Object httpMethod) {\n        return null;\n    }\n\n    Object createDelegate(PathMappedEndpoints endpoints, RequestMatcherFactory requestMatcherFactory,\n            RequestMatcherProvider matcherProvider) {\n        Set<String> paths = this.endpoints.stream()\n                .filter(java.util.Objects::nonNull)\n                .flatMap((endpointId) -> endpoints.getAdditionalPaths(this.webServerNamespace, endpointId).stream())\n                .collect(java.util.stream.Collectors.toCollection(java.util.LinkedHashSet::new));\n        return getDelegateMatchers(requestMatcherFactory, matcherProvider, paths, this.httpMethod);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "cloudfoundry-base-path-matcher-without-catchall-negative",
			rulePath:   "java/cwe-863-incorrect-authorization/java-cloudfoundry-base-path-matcher-without-catchall.sf",
			fileName:   "CloudFoundryMatcherConfigSafe.java",
			code:       "class Config {\n    static final String BASE_PATH = \"/cfApplication\";\n\n    Object pathMatcher(String path) { return null; }\n\n    Object getRequestMatcher() {\n        return pathMatcher(BASE_PATH + \"/**\");\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "cloudfoundry-base-path-matcher-without-catchall-positive",
			rulePath:   "java/cwe-863-incorrect-authorization/java-cloudfoundry-base-path-matcher-without-catchall.sf",
			fileName:   "CloudFoundryMatcherConfig.java",
			code:       "class PathMappedEndpoints {\n    void getAllPaths() {\n    }\n}\n\nclass Config {\n    static final String BASE_PATH = \"/cfApplication\";\n\n    Object pathMatcher(String path) { return null; }\n\n    Object getRequestMatcher(PathMappedEndpoints endpoints) {\n        endpoints.getAllPaths().forEach((path) -> pathMatcher(path + \"/**\"));\n        pathMatcher(BASE_PATH);\n        pathMatcher(BASE_PATH + \"/\");\n        return null;\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "document-builder-factory-unsafe-negative",
			rulePath:   "java/cwe-611-xxe/java-document-builder-factory-unsafe.sf",
			fileName:   "SafeXmlPayloadConverter.java",
			code:       "class DocumentBuilderFactoryUtils {\n    static Factory newInstance() { return null; }\n}\n\nclass InputStream {\n}\n\nclass Factory {\n    void setFeature(String feature, boolean flag) {\n    }\n    void setXIncludeAware(boolean flag) {\n    }\n    void setExpandEntityReferences(boolean flag) {\n    }\n    Builder newDocumentBuilder() { return null; }\n}\n\nclass Builder {\n    Object parse(Object input) { return null; }\n}\n\nclass SafeXmlPayloadConverter {\n    private final Factory documentBuilderFactory = DocumentBuilderFactoryUtils.newInstance();\n\n    Builder getDocumentBuilder() {\n        this.documentBuilderFactory.setFeature(\"disallow-doctype-decl\", true);\n        this.documentBuilderFactory.setXIncludeAware(false);\n        this.documentBuilderFactory.setExpandEntityReferences(false);\n        return this.documentBuilderFactory.newDocumentBuilder();\n    }\n\n    Object sourceToInputSource(Object source) { return source; }\n\n    Object convertToDocument(Object input) {\n        Object object = input;\n        if (object instanceof String) {\n            return getDocumentBuilder().parse(object);\n        }\n        else if (object instanceof InputStream) {\n            return getDocumentBuilder().parse(object);\n        }\n        else if (object instanceof byte[]) {\n            return getDocumentBuilder().parse(object);\n        }\n        return getDocumentBuilder().parse(sourceToInputSource(object));\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "document-builder-factory-unsafe-positive",
			rulePath:   "java/cwe-611-xxe/java-document-builder-factory-unsafe.sf",
			fileName:   "DefaultXmlPayloadConverter.java",
			code:       "class DocumentBuilderFactoryUtils {\n    static Factory newInstance() { return null; }\n}\n\nclass InputStream {\n}\n\nclass Factory {\n    Builder newDocumentBuilder() { return null; }\n}\n\nclass Builder {\n    Object parse(Object input) { return null; }\n}\n\nclass DefaultXmlPayloadConverter {\n    private final Factory documentBuilderFactory = DocumentBuilderFactoryUtils.newInstance();\n\n    Builder getDocumentBuilder() {\n        return this.documentBuilderFactory.newDocumentBuilder();\n    }\n\n    Object sourceToInputSource(Object source) { return source; }\n\n    Object convertToDocument(Object input) {\n        Object object = input;\n        if (object instanceof String) {\n            return getDocumentBuilder().parse(object);\n        }\n        else if (object instanceof InputStream) {\n            return getDocumentBuilder().parse(object);\n        }\n        else if (object instanceof byte[]) {\n            return getDocumentBuilder().parse(object);\n        }\n        return getDocumentBuilder().parse(sourceToInputSource(object));\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "empty-deserialization-allowlist-bypass-negative",
			rulePath:   "java/cwe-502-untrusted-unserialization/java-empty-deserialization-allowlist-bypass.sf",
			fileName:   "AllowlistVerifierSafe.java",
			code:       "import java.util.Set;\n\nclass ObjectUtils {\n    static boolean isEmpty(Object value) { return value == null; }\n}\n\nclass PatternMatchUtils {\n    static boolean simpleMatch(String pattern, String className) { return false; }\n}\n\nclass SecurityException extends RuntimeException {\n    SecurityException(String message) { super(message); }\n}\n\nclass AllowlistVerifier {\n    static void checkAllowedList(Class<?> clazz, Set<String> patterns) {\n        String className = clazz.getName();\n        for (String pattern : patterns) {\n            if (PatternMatchUtils.simpleMatch(pattern, className)) {\n                return;\n            }\n        }\n        throw new SecurityException(\"Attempt to deserialize unauthorized \" + clazz);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "empty-deserialization-allowlist-bypass-positive",
			rulePath:   "java/cwe-502-untrusted-unserialization/java-empty-deserialization-allowlist-bypass.sf",
			fileName:   "AllowlistVerifier.java",
			code:       "import java.util.Set;\n\nclass ObjectUtils {\n    static boolean isEmpty(Object value) { return value == null; }\n}\n\nclass PatternMatchUtils {\n    static boolean simpleMatch(String pattern, String className) { return false; }\n}\n\nclass SecurityException extends RuntimeException {\n    SecurityException(String message) { super(message); }\n}\n\nclass AllowlistVerifier {\n    static void checkAllowedList(Class<?> clazz, Set<String> patterns) {\n        if (ObjectUtils.isEmpty(patterns)) {\n            return;\n        }\n        String className = clazz.getName();\n        for (String pattern : patterns) {\n            if (PatternMatchUtils.simpleMatch(pattern, className)) {\n                return;\n            }\n        }\n        throw new SecurityException(\"Attempt to deserialize unauthorized \" + clazz);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "filter-converter-raw-string-value-negative",
			rulePath:   "java/cwe-74-improper-neutralization-of-special-elements/java-filter-converter-raw-string-value.sf",
			fileName:   "AbstractFilterExpressionConverterSafe.java",
			code:       "abstract class AbstractFilterExpressionConverter {\n    protected static void emitJsonValue(Object value, StringBuilder context) {\n    }\n\n    protected void doSingleValue(Object value, StringBuilder context) {\n        emitJsonValue(value, context);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "filter-converter-raw-string-value-positive",
			rulePath:   "java/cwe-74-improper-neutralization-of-special-elements/java-filter-converter-raw-string-value.sf",
			fileName:   "AbstractFilterExpressionConverter.java",
			code:       "abstract class AbstractFilterExpressionConverter {\n    protected void doSingleValue(Object value, StringBuilder context) {\n        if (value instanceof String) {\n            context.append(String.format(\"\\\"%s\\\"\", value));\n        }\n        else {\n            context.append(value);\n        }\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "filter-expression-converted-to-spel-negative",
			rulePath:   "java/cwe-94-code-injection/java-filter-expression-converted-to-spel.sf",
			fileName:   "SimpleVectorStoreSafe.java",
			code:       "class SearchRequest {\n    Object getFilterExpression() { return null; }\n}\n\nclass Evaluator {\n    boolean evaluate(Object expression, Object metadata) { return true; }\n}\n\nclass SimpleVectorStore {\n    private final Evaluator filterExpressionEvaluator = new Evaluator();\n\n    boolean doFilterPredicate(SearchRequest request, Object metadata) {\n        return this.filterExpressionEvaluator.evaluate(request.getFilterExpression(), metadata);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "filter-expression-converted-to-spel-positive",
			rulePath:   "java/cwe-94-code-injection/java-filter-expression-converted-to-spel.sf",
			fileName:   "SimpleVectorStore.java",
			code:       "class SearchRequest {\n    Object getFilterExpression() { return null; }\n}\n\nclass StandardEvaluationContext {\n    void setVariable(String name, Object value) {\n    }\n}\n\nclass Parser {\n    Parsed parseExpression(String expression) { return null; }\n}\n\nclass Parsed {\n    Boolean getValue(StandardEvaluationContext context, Class<Boolean> clazz) { return Boolean.TRUE; }\n}\n\nclass Converter {\n    String convertExpression(Object expression) { return \"\"; }\n}\n\nclass SimpleVectorStore {\n    private final Parser expressionParser = new Parser();\n    private final Converter filterExpressionConverter = new Converter();\n\n    boolean doFilterPredicate(SearchRequest request, Object metadata) {\n        StandardEvaluationContext context = new StandardEvaluationContext();\n        context.setVariable(\"metadata\", metadata);\n        return this.expressionParser\n                .parseExpression(this.filterExpressionConverter.convertExpression(request.getFilterExpression()))\n                .getValue(context, Boolean.class);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "jar-security-info-without-entry-content-match-negative",
			rulePath:   "java/cwe-347-improper-verification-of-cryptographic-signature/java-jar-security-info-without-entry-content-match.sf",
			fileName:   "SecurityInfoSafe.java",
			code:       "class JarEntriesStream {\n    JarEntriesStream(Object in) {\n    }\n\n    boolean matches(boolean directory, int size, int compressionMethod, Object supplier) { return true; }\n}\n\nclass SecurityInfo {\n    private static Object load(Object content) throws Exception {\n        JarEntriesStream entries = new JarEntriesStream(content);\n        entries.matches(false, 0, 0, null);\n        return entries;\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "jar-security-info-without-entry-content-match-positive",
			rulePath:   "java/cwe-347-improper-verification-of-cryptographic-signature/java-jar-security-info-without-entry-content-match.sf",
			fileName:   "SecurityInfo.java",
			code:       "class ZipContent {\n    Entry getEntry(String name) { return null; }\n    Object openRawZipData() { return null; }\n\n    static class Entry {\n        int getLookupIndex() { return 0; }\n    }\n}\n\nclass JarInputStream {\n    JarInputStream(Object in) {\n    }\n}\n\nclass JarEntry {\n    Object[] getCertificates() { return null; }\n    Object[] getCodeSigners() { return null; }\n    String getName() { return \"\"; }\n}\n\nclass SecurityInfo {\n    private static Object load(ZipContent content) throws Exception {\n        try (JarInputStream in = new JarInputStream(content.openRawZipData())) {\n            JarEntry jarEntry = null;\n            Object[] certificates = jarEntry.getCertificates();\n            Object[] codeSigners = jarEntry.getCodeSigners();\n            ZipContent.Entry contentEntry = content.getEntry(jarEntry.getName());\n            return contentEntry;\n        }\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "logout-handler-missing-empty-context-save-negative",
			rulePath:   "java/cwe-287-improper-authentication/java-logout-handler-missing-empty-context-save.sf",
			fileName:   "SecurityContextLogoutHandlerSafe.java",
			code:       "class SecurityContext {\n    void setAuthentication(Object authentication) {\n    }\n}\n\nclass SecurityContextRepository {\n    void saveContext(SecurityContext context, Object request, Object response) {\n    }\n}\n\nclass SecurityContextHolderStrategy {\n    SecurityContext getContext() { return null; }\n    SecurityContext createEmptyContext() { return null; }\n    void clearContext() {\n    }\n}\n\nclass SecurityContextLogoutHandler {\n    private final SecurityContextHolderStrategy securityContextHolderStrategy = new SecurityContextHolderStrategy();\n    private final SecurityContextRepository securityContextRepository = new SecurityContextRepository();\n\n    void logout(Object request, Object response) {\n        SecurityContext context = this.securityContextHolderStrategy.getContext();\n        this.securityContextHolderStrategy.clearContext();\n        context.setAuthentication(null);\n        SecurityContext emptyContext = this.securityContextHolderStrategy.createEmptyContext();\n        this.securityContextRepository.saveContext(emptyContext, request, response);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "logout-handler-missing-empty-context-save-positive",
			rulePath:   "java/cwe-287-improper-authentication/java-logout-handler-missing-empty-context-save.sf",
			fileName:   "SecurityContextLogoutHandler.java",
			code:       "class SecurityContext {\n    void setAuthentication(Object authentication) {\n    }\n}\n\nclass SecurityContextHolderStrategy {\n    SecurityContext getContext() { return null; }\n    void clearContext() {\n    }\n}\n\nclass SecurityContextLogoutHandler {\n    private final SecurityContextHolderStrategy securityContextHolderStrategy = new SecurityContextHolderStrategy();\n\n    void logout() {\n        SecurityContext context = this.securityContextHolderStrategy.getContext();\n        this.securityContextHolderStrategy.clearContext();\n        context.setAuthentication(null);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "mariadb-filter-string-without-sql-escape-negative",
			rulePath:   "java/cwe-89-sql-injection/java-mariadb-filter-string-without-sql-escape.sf",
			fileName:   "MariaDBFilterExpressionConverterSafe.java",
			code:       "class MariaDBFilterExpressionConverter {\n    protected static void emitSqlString(String value, StringBuilder context) {\n    }\n\n    protected void doSingleValue(Object value, StringBuilder context) {\n        if (value instanceof String stringValue) {\n            emitSqlString(stringValue, context);\n        }\n        else {\n            context.append(value);\n        }\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "mariadb-filter-string-without-sql-escape-positive",
			rulePath:   "java/cwe-89-sql-injection/java-mariadb-filter-string-without-sql-escape.sf",
			fileName:   "MariaDBFilterExpressionConverter.java",
			code:       "class MariaDBFilterExpressionConverter {\n    protected void doSingleValue(Object value, StringBuilder context) {\n        if (value instanceof String) {\n            context.append(String.format(\"\\'%s\\'\", value));\n        }\n        else {\n            context.append(value);\n        }\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "neo4j-filter-key-without-sanitize-negative",
			rulePath:   "java/cwe-74-improper-neutralization-of-special-elements/java-neo4j-filter-key-without-sanitize.sf",
			fileName:   "Neo4jVectorFilterExpressionConverterSafe.java",
			code:       "class Key {\n    String key() { return null; }\n}\n\nclass SchemaNames {\n    static String sanitize(String value, boolean flag) { return value; }\n}\n\nclass Neo4jVectorFilterExpressionConverter {\n    protected void doKey(Key key, StringBuilder context) {\n        String sanitized = SchemaNames.sanitize(\"metadata.\" + key.key(), true);\n        context.append(\"node.\").append(sanitized);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "neo4j-filter-key-without-sanitize-positive",
			rulePath:   "java/cwe-74-improper-neutralization-of-special-elements/java-neo4j-filter-key-without-sanitize.sf",
			fileName:   "Neo4jVectorFilterExpressionConverter.java",
			code:       "abstract class Key {\n    String key() { return null; }\n}\n\nclass Neo4jVectorFilterExpressionConverter {\n    protected void doKey(Key key, StringBuilder context) {\n        context.append(\"node.\").append(\"`metadata.\").append(key.key().replace(\"\\\"\", \"\")).append(\"`\");\n    }\n}\n",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "oauth2-authorization-request-session-accumulation-httpsession-positive",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-oauth2-authorization-request-session-accumulation.sf",
			fileName:   "AuthorizationRequestRepository.java",
			code:       "import java.util.HashMap;\nimport java.util.Map;\n\nclass OAuth2AuthorizationRequest {\n    String getState() { return \"state\"; }\n}\n\ninterface HttpSession {\n    void setAttribute(String name, Object value);\n}\n\ninterface HttpServletRequest {\n    HttpSession getSession();\n}\n\ninterface HttpServletResponse {\n}\n\nclass AuthorizationRequestRepository {\n    private final String sessionAttributeName = \"authz\";\n\n    Map<String, OAuth2AuthorizationRequest> getAuthorizationRequests(HttpServletRequest request) {\n        return new HashMap<>();\n    }\n\n    void saveAuthorizationRequest(OAuth2AuthorizationRequest authorizationRequest, HttpServletRequest request,\n            HttpServletResponse response) {\n        String state = authorizationRequest.getState();\n        Map<String, OAuth2AuthorizationRequest> authorizationRequests = this.getAuthorizationRequests(request);\n        authorizationRequests.put(state, authorizationRequest);\n        request.getSession().setAttribute(this.sessionAttributeName, authorizationRequests);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "oauth2-authorization-request-session-accumulation-negative",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-oauth2-authorization-request-session-accumulation.sf",
			fileName:   "AuthorizationRequestRepositorySafe.java",
			code:       "class OAuth2AuthorizationRequest {\n}\n\ninterface HttpSession {\n    void setAttribute(String name, Object value);\n}\n\ninterface HttpServletRequest {\n    HttpSession getSession();\n}\n\ninterface HttpServletResponse {\n}\n\nclass AuthorizationRequestRepository {\n    private final String sessionAttributeName = \"authz\";\n\n    void saveAuthorizationRequest(OAuth2AuthorizationRequest authorizationRequest, HttpServletRequest request,\n            HttpServletResponse response) {\n        request.getSession().setAttribute(this.sessionAttributeName, authorizationRequest);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "oauth2-authorization-request-session-accumulation-websession-positive",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-oauth2-authorization-request-session-accumulation.sf",
			fileName:   "ReactiveAuthorizationRequestRepository.java",
			code:       "import java.util.HashMap;\nimport java.util.Map;\n\nclass OAuth2AuthorizationRequest {\n    String getState() { return \"state\"; }\n}\n\nclass Mono<T> {\n    Mono<T> doOnNext(java.util.function.Consumer<T> c) { return this; }\n    Mono<Void> then() { return new Mono<Void>(); }\n}\n\nclass ServerWebExchange {\n}\n\nclass AuthorizationRequestRepository {\n    private final String sessionAttributeName = \"authz\";\n\n    Mono<Map<String, Object>> getSessionAttributes(ServerWebExchange exchange) {\n        return new Mono<Map<String, Object>>();\n    }\n\n    Map<String, OAuth2AuthorizationRequest> getAuthorizationRequests(Map<String, Object> sessionAttrs) {\n        return new HashMap<>();\n    }\n\n    Mono<Void> saveAuthorizationRequest(OAuth2AuthorizationRequest authorizationRequest, ServerWebExchange exchange) {\n        return getSessionAttributes(exchange)\n                .doOnNext((sessionAttrs) -> {\n                    Map<String, OAuth2AuthorizationRequest> authorizationRequests = this.getAuthorizationRequests(sessionAttrs);\n                    authorizationRequests.put(authorizationRequest.getState(), authorizationRequest);\n                    sessionAttrs.put(this.sessionAttributeName, authorizationRequests);\n                })\n                .then();\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "oxm-unmarshaller-untrusted-source-factory-positive",
			rulePath:   "java/cwe-611-xxe/java-oxm-unmarshaller-untrusted-source.sf",
			fileName:   "RuntimeSourceTransformer.java",
			code:       "interface SourceFactory {\n    Object createSource(Object payload);\n}\n\ninterface Unmarshaller {\n    Object unmarshal(Object source) throws java.io.IOException;\n}\n\nclass RuntimeSourceTransformer {\n    private final Unmarshaller unmarshaller = null;\n    private final SourceFactory sourceFactory = null;\n\n    Object transformPayload(Object payload) throws Exception {\n        Object source = this.sourceFactory.createSource(payload);\n        return this.unmarshaller.unmarshal(source);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "oxm-unmarshaller-untrusted-source-negative",
			rulePath:   "java/cwe-611-xxe/java-oxm-unmarshaller-untrusted-source.sf",
			fileName:   "SafeTransformer.java",
			code:       "class DOMSource {\n    DOMSource(Object document) {\n    }\n}\n\ninterface Unmarshaller {\n    Object unmarshal(Object source) throws java.io.IOException;\n}\n\nclass SafeTransformer {\n    private final Unmarshaller unmarshaller = null;\n\n    Object transformPayload() throws Exception {\n        Object source = new DOMSource(new Object());\n        return this.unmarshaller.unmarshal(source);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "oxm-unmarshaller-untrusted-source-positive",
			rulePath:   "java/cwe-611-xxe/java-oxm-unmarshaller-untrusted-source.sf",
			fileName:   "UnmarshallingTransformer.java",
			code:       "class StringSource {\n    StringSource(String payload) {\n    }\n}\n\ninterface Unmarshaller {\n    Object unmarshal(Object source) throws java.io.IOException;\n}\n\nclass UnmarshallingTransformer {\n    private final Unmarshaller unmarshaller = null;\n\n    Object transformPayload(String payload) throws Exception {\n        Object source = new StringSource(payload);\n        return this.unmarshaller.unmarshal(source);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "recursive-property-path-parser-depth-limit-negative",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-recursive-property-path-parser-without-depth-limit.sf",
			fileName:   "PropertyPathParserSafe.java",
			code:       "import java.util.List;\nimport java.util.regex.Matcher;\nimport java.util.regex.Pattern;\n\nclass PropertyPathParser {\n    static Object create(String source, Object type, String addTail, List<Object> base) {\n        if (base.size() > 1000) {\n            throw new IllegalArgumentException(\"depth exceeded\");\n        }\n        Pattern pattern = Pattern.compile(\"\\\\p{Lu}+\\\\p{Ll}*$\");\n        Matcher matcher = pattern.matcher(source);\n        if (matcher.find()) {\n            return create(\"head\", type, \"tail\" + addTail, base);\n        }\n        return source;\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "recursive-property-path-parser-depth-limit-positive",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-recursive-property-path-parser-without-depth-limit.sf",
			fileName:   "PropertyPathParser.java",
			code:       "import java.util.List;\nimport java.util.regex.Matcher;\nimport java.util.regex.Pattern;\n\nclass PropertyPathParser {\n    static Object create(String source, Object type, String addTail, List<Object> base) {\n        Pattern pattern = Pattern.compile(\"\\\\p{Lu}+\\\\p{Ll}*$\");\n        Matcher matcher = pattern.matcher(source);\n        if (matcher.find()) {\n            return create(\"head\", type, \"tail\" + addTail, base);\n        }\n        return source;\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "redis-filter-value-without-query-escape-negative",
			rulePath:   "java/cwe-74-improper-neutralization-of-special-elements/java-redis-filter-value-without-query-escape.sf",
			fileName:   "RedisFilterExpressionConverterSafe.java",
			code:       "import java.util.List;\n\nclass Expression {\n}\n\nclass Value {\n    Object value() { return null; }\n}\n\nclass RediSearchUtil {\n    static String escapeQuery(String value) { return value; }\n}\n\nclass RedisFilterExpressionConverter {\n    private String escapeTagValue(String value) { return value; }\n\n    private String tagStringValue(Expression expression, Value value) {\n        String delimiter = \" | \";\n        if (value.value() instanceof List<?> list) {\n            return list.stream().map(String::valueOf).map(this::escapeTagValue).toList().toString();\n        }\n        return escapeTagValue(String.valueOf(value.value()));\n    }\n\n    private String textStringValue(Expression expression, Value value) {\n        if (value.value() instanceof List<?> list) {\n            return list.stream().map(String::valueOf).map(RediSearchUtil::escapeQuery).toList().toString();\n        }\n        return RediSearchUtil.escapeQuery(String.valueOf(value.value()));\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "redis-filter-value-without-query-escape-positive",
			rulePath:   "java/cwe-74-improper-neutralization-of-special-elements/java-redis-filter-value-without-query-escape.sf",
			fileName:   "RedisFilterExpressionConverter.java",
			code:       "import java.util.List;\n\nclass Expression {\n}\n\nclass Value {\n    Object value() { return null; }\n}\n\nclass RedisFilterExpressionConverter {\n    private Object stringValue(Expression expression, Value value) {\n        String delimiter = \" | \";\n        if (value.value() instanceof List<?> list) {\n            return String.join(delimiter, list.stream().map(String::valueOf).toList());\n        }\n        return value.value();\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "regex-etag-header-parsing-audit-negative",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-regex-etag-header-parsing-audit.sf",
			fileName:   "ETagParsingSafe.java",
			code:       "class ETag {\n    static void parse(String value) {\n    }\n}\n\nclass HttpHeaders {\n    void getETagValuesAsList(String value) {\n        ETag.parse(value);\n    }\n}\n\nclass ServletWebRequest {\n    void matchRequestedETags(String value) {\n        ETag.parse(value);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "regex-etag-header-parsing-audit-positive",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-regex-etag-header-parsing-audit.sf",
			fileName:   "ETagParsing.java",
			code:       "class Matcher {\n    boolean find() { return false; }\n}\n\nclass Pattern {\n    Matcher matcher(String value) { return null; }\n}\n\nclass HttpHeaders {\n    private static final Pattern ETAG_HEADER_VALUE_PATTERN = new Pattern();\n\n    void getETagValuesAsList(String value) {\n        Matcher matcher = ETAG_HEADER_VALUE_PATTERN.matcher(value);\n        while (matcher.find()) {\n        }\n    }\n}\n\nclass ServletWebRequest {\n    private static final Pattern ETAG_HEADER_VALUE_PATTERN = new Pattern();\n\n    void matchRequestedETags(String value) {\n        Matcher etagMatcher = ETAG_HEADER_VALUE_PATTERN.matcher(value);\n        while (etagMatcher.find()) {\n        }\n    }\n}",
			wantAlerts: map[string]int{"risk": 2},
		},
		{
			name:       "remote-media-fetch-without-strict-url-validation-negative",
			rulePath:   "java/cwe-918-ssrf/java-remote-media-fetch-without-strict-url-validation.sf",
			fileName:   "BedrockProxyChatModelSafe.java",
			code:       "import java.net.URI;\nimport java.net.URL;\n\nclass Media {\n    Object getData() { return null; }\n}\n\nclass MediaFetcher {\n    byte[] fetch(URI uri) { return null; }\n}\n\nclass BedrockProxyChatModel {\n    private final MediaFetcher mediaFetcher = new MediaFetcher();\n\n    Object mapMediaToContentBlock(Media media) throws Exception {\n        if (media.getData() instanceof String text) {\n            return this.mediaFetcher.fetch(URI.create(text));\n        }\n        else if (media.getData() instanceof URL url) {\n            return this.mediaFetcher.fetch(url.toURI());\n        }\n        return null;\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "remote-media-fetch-without-strict-url-validation-positive",
			rulePath:   "java/cwe-918-ssrf/java-remote-media-fetch-without-strict-url-validation.sf",
			fileName:   "BedrockProxyChatModel.java",
			code:       "import java.io.InputStream;\nimport java.net.URL;\n\nclass Media {\n    Object getData() { return null; }\n}\n\nclass BedrockProxyChatModel {\n    Object mapMediaToContentBlock(Media media) throws Exception {\n        if (media.getData() instanceof String text) {\n            URL url = new URL(text);\n            try (InputStream is = url.openConnection().getInputStream()) {\n                return is;\n            }\n        }\n        else if (media.getData() instanceof URL url) {\n            try (InputStream is = url.openConnection().getInputStream()) {\n                return is;\n            }\n        }\n        return null;\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "saml-response-signature-consistency-negative",
			rulePath:   "java/cwe-347-improper-verification-of-cryptographic-signature/java-saml-response-signature-consistency.sf",
			fileName:   "OpenSamlAuthenticationProviderSafe.java",
			code:       "class Provider {\n    java.util.List<Object> validateResponse(Object token, Object response) {\n        isSigned(response, java.util.List.of());\n        return java.util.List.of();\n    }\n\n    boolean isSigned(Object response, java.util.List<Object> assertions) { return true; }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "saml-response-signature-consistency-positive",
			rulePath:   "java/cwe-347-improper-verification-of-cryptographic-signature/java-saml-response-signature-consistency.sf",
			fileName:   "OpenSamlAuthenticationProvider.java",
			code:       "class Saml2AuthenticationToken {\n}\n\nclass Assertion {\n}\n\nclass Response {\n    java.util.List<Assertion> getAssertions() { return null; }\n}\n\nclass Provider {\n    Assertion validateSaml2Response(Saml2AuthenticationToken token, String recipient, Response samlResponse) {\n        boolean responseSigned = hasValidSignature(samlResponse, token);\n        for (Assertion a : samlResponse.getAssertions()) {\n            validateAssertion(recipient, a, token, !responseSigned);\n            return a;\n        }\n        return null;\n    }\n\n    boolean hasValidSignature(Response response, Saml2AuthenticationToken token) { return false; }\n\n    void validateAssertion(String recipient, Assertion assertion, Saml2AuthenticationToken token,\n            boolean signatureRequired) {\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "security-context-save-gated-negative",
			rulePath:   "java/cwe-287-improper-authentication/java-security-context-save-gated-by-contextsaved-flag.sf",
			fileName:   "RepositorySafe.java",
			code:       "class SecurityContext {\n}\n\nclass SaveContextOnUpdateOrErrorResponseWrapper {\n    void saveContext(SecurityContext context) {\n    }\n}\n\nclass Repository {\n    void saveContext(SecurityContext context, SaveContextOnUpdateOrErrorResponseWrapper responseWrapper) {\n        responseWrapper.saveContext(context);\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "security-context-save-gated-positive",
			rulePath:   "java/cwe-287-improper-authentication/java-security-context-save-gated-by-contextsaved-flag.sf",
			fileName:   "Repository.java",
			code:       "class SecurityContext {\n}\n\nclass SaveContextOnUpdateOrErrorResponseWrapper {\n    boolean isContextSaved() { return false; }\n    void saveContext(SecurityContext context) {\n    }\n}\n\nclass Repository {\n    void saveContext(SecurityContext context, SaveContextOnUpdateOrErrorResponseWrapper responseWrapper) {\n        if (!responseWrapper.isContextSaved()) {\n            responseWrapper.saveContext(context);\n        }\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "spel-array-construction-without-size-limit-negative",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-spel-array-construction-without-size-limit.sf",
			fileName:   "SafeArrayConstructor.java",
			code:       "import java.lang.reflect.Array;\n\nclass ExpressionUtils {\n    static int toInt(Object converter, Object value) {\n        return 0;\n    }\n}\n\nclass State {\n    Class<?> findType(String type) {\n        return Object.class;\n    }\n}\n\nclass SafeArrayConstructor {\n    Object createArray(State state, Object converter, Object value, String type) {\n        int arraySize = ExpressionUtils.toInt(converter, value);\n        Class<?> componentType = state.findType(type);\n        checkNumElements(arraySize);\n        return Array.newInstance(componentType, arraySize);\n    }\n\n    void checkNumElements(int arraySize) {\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "spel-array-construction-without-size-limit-positive",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-spel-array-construction-without-size-limit.sf",
			fileName:   "UnsafeArrayConstructor.java",
			code:       "import java.lang.reflect.Array;\n\nclass ExpressionUtils {\n    static int toInt(Object converter, Object value) {\n        return 0;\n    }\n}\n\nclass State {\n    Class<?> findType(String type) {\n        return Object.class;\n    }\n}\n\nclass UnsafeArrayConstructor {\n    Object createArray(State state, Object converter, Object value, String type) {\n        int arraySize = ExpressionUtils.toInt(converter, value);\n        Class<?> componentType = state.findType(type);\n        return Array.newInstance(componentType, arraySize);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "spel-expression-length-without-limit-negative",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-spel-expression-length-without-limit.sf",
			fileName:   "InternalSpelExpressionParserSafe.java",
			code:       "class Tokenizer {\n    Tokenizer(String expressionString) {\n    }\n}\n\nclass InternalSpelExpressionParser {\n    Object doParseExpression(String expressionString, Object context) {\n        checkExpressionLength(expressionString);\n        Tokenizer tokenizer = new Tokenizer(expressionString);\n        return tokenizer;\n    }\n\n    void checkExpressionLength(String expressionString) {\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "spel-expression-length-without-limit-positive",
			rulePath:   "java/cwe-400-uncontrolled-resource-consumption/java-spel-expression-length-without-limit.sf",
			fileName:   "InternalSpelExpressionParser.java",
			code:       "class Tokenizer {\n    Tokenizer(String expressionString) {\n    }\n}\n\nclass InternalSpelExpressionParser {\n    Object doParseExpression(String expressionString, Object context) {\n        Tokenizer tokenizer = new Tokenizer(expressionString);\n        return tokenizer;\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "spring-message-header-expression-fixed-negative",
			rulePath:   "java/cwe-94-code-injection/java-spring-message-header-expression-evaluation.sf",
			fileName:   "FixedExpressionRegistry.java",
			code:       "import org.springframework.expression.Expression;\nimport org.springframework.expression.spel.standard.SpelExpressionParser;\n\nclass FixedExpressionRegistry {\n    private final SpelExpressionParser expressionParser = new SpelExpressionParser();\n\n    Expression safe() {\n        return this.expressionParser.parseExpression(\"headers.foo == 'bar'\");\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "spring-message-header-expression-forwarding-positive",
			rulePath:   "java/cwe-94-code-injection/java-spring-message-header-expression-evaluation.sf",
			fileName:   "NativeHeaderExpressionController.java",
			code:       "import org.springframework.expression.Expression;\nimport org.springframework.expression.ExpressionParser;\nimport org.springframework.expression.spel.standard.SpelExpressionParser;\n\nclass MessageHeaders {\n}\n\nclass NativeMessageHeaderAccessor {\n    static String getFirstNativeHeader(String name, MessageHeaders headers) {\n        return null;\n    }\n}\n\nclass NativeHeaderExpressionController {\n    private final ExpressionParser parser = new SpelExpressionParser();\n\n    Expression bad(MessageHeaders headers, String selectorHeaderName) {\n        String expression = NativeMessageHeaderAccessor.getFirstNativeHeader(selectorHeaderName, headers);\n        return this.parser.parseExpression(expression);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "spring-message-header-expression-native-header-positive",
			rulePath:   "java/cwe-94-code-injection/java-spring-message-header-expression-evaluation.sf",
			fileName:   "SelectorExpressionRegistry.java",
			code:       "import org.springframework.expression.Expression;\nimport org.springframework.expression.spel.standard.SpelExpressionParser;\n\nclass MessageHeaders {\n}\n\nclass NativeMessageHeaderAccessor {\n    static String getFirstNativeHeader(String name, MessageHeaders headers) {\n        return null;\n    }\n}\n\nclass SelectorExpressionRegistry {\n    private final SpelExpressionParser expressionParser = new SpelExpressionParser();\n\n    Expression bad(MessageHeaders headers, String selectorHeaderName) {\n        String selector = NativeMessageHeaderAccessor.getFirstNativeHeader(selectorHeaderName, headers);\n        return this.expressionParser.parseExpression(selector);\n    }\n}",
			wantAlerts: map[string]int{"risk": 1},
		},
		{
			name:       "spring-message-header-expression-readonly-negative",
			rulePath:   "java/cwe-94-code-injection/java-spring-message-header-expression-evaluation.sf",
			fileName:   "HeaderReadOnly.java",
			code:       "class MessageHeaders {\n}\n\nclass HeaderReadOnly {\n    String safe(MessageHeaders headers) {\n        return null;\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "spring-model-and-view-ssti-constructor-positive",
			rulePath:   "java/cwe-1336-ssti/java-spring-framework-model-controllable.sf",
			fileName:   "OrgConsoleController.java",
			code:       "import org.springframework.stereotype.Controller;\nimport org.springframework.web.bind.annotation.GetMapping;\nimport org.springframework.web.bind.annotation.RequestParam;\nimport org.springframework.web.servlet.ModelAndView;\n\n@Controller\npublic class OrgConsoleController {\n    @GetMapping(\"/edit\")\n    public ModelAndView edit(@RequestParam String id) {\n        return new ModelAndView(\"/admin/org\" + id + \"/edit.html\");\n    }\n}",
			wantAlerts: map[string]int{"filteredSource": 1},
		},
		{
			name:       "spring-model-and-view-ssti-negative",
			rulePath:   "java/cwe-1336-ssti/java-spring-framework-model-controllable.sf",
			fileName:   "OrgConsoleController.java",
			code:       "import org.springframework.stereotype.Controller;\nimport org.springframework.web.bind.annotation.GetMapping;\nimport org.springframework.web.bind.annotation.RequestParam;\nimport org.springframework.web.servlet.ModelAndView;\n\n@Controller\npublic class OrgConsoleController {\n    @GetMapping(\"/edit\")\n    public ModelAndView edit(@RequestParam String id) {\n        ModelAndView view = new ModelAndView(\"/admin/org/edit.html\");\n        Object org = orgConsoleService.queryById(id);\n        view.addObject(\"org\", org);\n        return view;\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "spring-model-and-view-ssti-setview-positive",
			rulePath:   "java/cwe-1336-ssti/java-spring-framework-model-controllable.sf",
			fileName:   "OrgConsoleController.java",
			code:       "import org.springframework.stereotype.Controller;\nimport org.springframework.web.bind.annotation.GetMapping;\nimport org.springframework.web.bind.annotation.RequestParam;\nimport org.springframework.web.servlet.ModelAndView;\n\n@Controller\npublic class OrgConsoleController {\n    @GetMapping(\"/edit\")\n    public ModelAndView edit(@RequestParam String id) {\n        ModelAndView view = new ModelAndView();\n        view.setViewName(\"/admin/org\" + id + \"/edit.html\");\n        return view;\n    }\n}",
			wantAlerts: map[string]int{"filteredSource": 1},
		},
		{
			name:       "spring-response-body-xss-filtered-positive",
			rulePath:   "java/cwe-79-xss/java-spring-response-body-xss.sf",
			fileName:   "XSSController.java",
			code:       "import org.springframework.web.bind.annotation.*;\nimport org.springframework.web.util.HtmlUtils;\n\n@RestController\n@RequestMapping(\"/xss\")\npublic class XSSController {\n    @GetMapping(\"/echo\")\n    public String echo(@RequestParam(\"input\") String input) {\n        return \"hello \" + HtmlUtils.htmlEscape(input);\n    }\n}",
			wantAlerts: map[string]int{"filteredSink": 1, "withoutCall": 0},
		},
		{
			name:       "spring-response-body-xss-map-return-negative",
			rulePath:   "java/cwe-79-xss/java-spring-response-body-xss.sf",
			fileName:   "XSSController.java",
			code:       "import java.util.HashMap;\nimport java.util.Map;\nimport org.springframework.web.bind.annotation.*;\n\n@RestController\n@RequestMapping(\"/xss\")\npublic class XSSController {\n    @GetMapping(\"/echo\")\n    public Map<String, Object> echo(@RequestParam(\"input\") String input) {\n        Map<String, Object> modelMap = new HashMap<>();\n        modelMap.put(\"payload\", input);\n        modelMap.put(\"status\", \"ok\");\n        return modelMap;\n    }\n}",
			wantAlerts: map[string]int{"total": 0},
		},
		{
			name:       "spring-response-body-xss-string-return-positive",
			rulePath:   "java/cwe-79-xss/java-spring-response-body-xss.sf",
			fileName:   "XSSController.java",
			code:       "import org.springframework.web.bind.annotation.*;\n\n@RestController\n@RequestMapping(\"/xss\")\npublic class XSSController {\n    @GetMapping(\"/echo\")\n    public String echo(@RequestParam(\"input\") String input) {\n        return \"hello \" + input;\n    }\n}",
			wantAlerts: map[string]int{"withoutCall": 1, "filteredSink": 0},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rule := loadBuiltinRule(t, c.rulePath)
			counts := runJavaBuiltinRule(t, rule, c.fileName, c.code)
			for variable, expected := range c.wantAlerts {
				actual := counts.ByVariable[variable]
				if variable == "total" {
					actual = counts.Total
				}
				if expected == 0 {
					require.Equalf(t, 0, actual, "case %s should not report %s alerts", c.name, variable)
					continue
				}
				require.GreaterOrEqualf(t, actual, expected, "case %s should report at least %d %s alerts", c.name, expected, variable)
			}
		})
	}
}

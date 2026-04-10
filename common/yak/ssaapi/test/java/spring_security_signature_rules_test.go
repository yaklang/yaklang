package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJarSecurityInfoWithoutEntryContentMatchRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-347-improper-verification-of-cryptographic-signature/java-jar-security-info-without-entry-content-match.sf")
	code := `
class ZipContent {
    Entry getEntry(String name) { return null; }
    Object openRawZipData() { return null; }

    static class Entry {
        int getLookupIndex() { return 0; }
    }
}

class JarInputStream {
    JarInputStream(Object in) {
    }
}

class JarEntry {
    Object[] getCertificates() { return null; }
    Object[] getCodeSigners() { return null; }
    String getName() { return ""; }
}

class SecurityInfo {
    private static Object load(ZipContent content) throws Exception {
        try (JarInputStream in = new JarInputStream(content.openRawZipData())) {
            JarEntry jarEntry = null;
            Object[] certificates = jarEntry.getCertificates();
            Object[] codeSigners = jarEntry.getCodeSigners();
            ZipContent.Entry contentEntry = content.getEntry(jarEntry.getName());
            return contentEntry;
        }
    }
}
`
	counts := runJavaRule(t, rule, "SecurityInfo.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestJarSecurityInfoWithoutEntryContentMatchRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-347-improper-verification-of-cryptographic-signature/java-jar-security-info-without-entry-content-match.sf")
	code := `
class JarEntriesStream {
    JarEntriesStream(Object in) {
    }

    boolean matches(boolean directory, int size, int compressionMethod, Object supplier) { return true; }
}

class SecurityInfo {
    private static Object load(Object content) throws Exception {
        JarEntriesStream entries = new JarEntriesStream(content);
        entries.matches(false, 0, 0, null);
        return entries;
    }
}
`
	counts := runJavaRule(t, rule, "SecurityInfoSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

func TestSamlResponseSignatureConsistencyRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-347-improper-verification-of-cryptographic-signature/java-saml-response-signature-consistency.sf")
	code := `
class Saml2AuthenticationToken {
}

class Assertion {
}

class Response {
    java.util.List<Assertion> getAssertions() { return null; }
}

class Provider {
    Assertion validateSaml2Response(Saml2AuthenticationToken token, String recipient, Response samlResponse) {
        boolean responseSigned = hasValidSignature(samlResponse, token);
        for (Assertion a : samlResponse.getAssertions()) {
            validateAssertion(recipient, a, token, !responseSigned);
            return a;
        }
        return null;
    }

    boolean hasValidSignature(Response response, Saml2AuthenticationToken token) { return false; }

    void validateAssertion(String recipient, Assertion assertion, Saml2AuthenticationToken token,
            boolean signatureRequired) {
    }
}
`
	counts := runJavaRule(t, rule, "OpenSamlAuthenticationProvider.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestSamlResponseSignatureConsistencyRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-347-improper-verification-of-cryptographic-signature/java-saml-response-signature-consistency.sf")
	code := `
class Provider {
    java.util.List<Object> validateResponse(Object token, Object response) {
        isSigned(response, java.util.List.of());
        return java.util.List.of();
    }

    boolean isSigned(Object response, java.util.List<Object> assertions) { return true; }
}
`
	counts := runJavaRule(t, rule, "OpenSamlAuthenticationProviderSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

func TestLogoutHandlerMissingEmptyContextSaveRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-287-improper-authentication/java-logout-handler-missing-empty-context-save.sf")
	code := `
class SecurityContext {
    void setAuthentication(Object authentication) {
    }
}

class SecurityContextHolderStrategy {
    SecurityContext getContext() { return null; }
    void clearContext() {
    }
}

class SecurityContextLogoutHandler {
    private final SecurityContextHolderStrategy securityContextHolderStrategy = new SecurityContextHolderStrategy();

    void logout() {
        SecurityContext context = this.securityContextHolderStrategy.getContext();
        this.securityContextHolderStrategy.clearContext();
        context.setAuthentication(null);
    }
}
`
	counts := runJavaRule(t, rule, "SecurityContextLogoutHandler.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestLogoutHandlerMissingEmptyContextSaveRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-287-improper-authentication/java-logout-handler-missing-empty-context-save.sf")
	code := `
class SecurityContext {
    void setAuthentication(Object authentication) {
    }
}

class SecurityContextRepository {
    void saveContext(SecurityContext context, Object request, Object response) {
    }
}

class SecurityContextHolderStrategy {
    SecurityContext getContext() { return null; }
    SecurityContext createEmptyContext() { return null; }
    void clearContext() {
    }
}

class SecurityContextLogoutHandler {
    private final SecurityContextHolderStrategy securityContextHolderStrategy = new SecurityContextHolderStrategy();
    private final SecurityContextRepository securityContextRepository = new SecurityContextRepository();

    void logout(Object request, Object response) {
        SecurityContext context = this.securityContextHolderStrategy.getContext();
        this.securityContextHolderStrategy.clearContext();
        context.setAuthentication(null);
        SecurityContext emptyContext = this.securityContextHolderStrategy.createEmptyContext();
        this.securityContextRepository.saveContext(emptyContext, request, response);
    }
}
`
	counts := runJavaRule(t, rule, "SecurityContextLogoutHandlerSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

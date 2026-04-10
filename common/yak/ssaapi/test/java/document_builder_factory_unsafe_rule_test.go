package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadDocumentBuilderFactoryUnsafeRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-611-xxe/java-document-builder-factory-unsafe.sf")
	if !ok {
		t.Skip("java-document-builder-factory-unsafe.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-document-builder-factory-unsafe.sf 内容为空")
	return content
}

func TestDocumentBuilderFactoryUnsafeRule_Positive(t *testing.T) {
	rule := loadDocumentBuilderFactoryUnsafeRule(t)
	code := `
class DocumentBuilderFactoryUtils {
    static Factory newInstance() { return null; }
}

class InputStream {
}

class Factory {
    Builder newDocumentBuilder() { return null; }
}

class Builder {
    Object parse(Object input) { return null; }
}

class DefaultXmlPayloadConverter {
    private final Factory documentBuilderFactory = DocumentBuilderFactoryUtils.newInstance();

    Builder getDocumentBuilder() {
        return this.documentBuilderFactory.newDocumentBuilder();
    }

    Object sourceToInputSource(Object source) { return source; }

    Object convertToDocument(Object input) {
        Object object = input;
        if (object instanceof String) {
            return getDocumentBuilder().parse(object);
        }
        else if (object instanceof InputStream) {
            return getDocumentBuilder().parse(object);
        }
        else if (object instanceof byte[]) {
            return getDocumentBuilder().parse(object);
        }
        return getDocumentBuilder().parse(sourceToInputSource(object));
    }
}
`
	counts := runJavaRule(t, rule, "DefaultXmlPayloadConverter.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestDocumentBuilderFactoryUnsafeRule_Negative(t *testing.T) {
	rule := loadDocumentBuilderFactoryUnsafeRule(t)
	code := `
class DocumentBuilderFactoryUtils {
    static Factory newInstance() { return null; }
}

class InputStream {
}

class Factory {
    void setFeature(String feature, boolean flag) {
    }
    void setXIncludeAware(boolean flag) {
    }
    void setExpandEntityReferences(boolean flag) {
    }
    Builder newDocumentBuilder() { return null; }
}

class Builder {
    Object parse(Object input) { return null; }
}

class SafeXmlPayloadConverter {
    private final Factory documentBuilderFactory = DocumentBuilderFactoryUtils.newInstance();

    Builder getDocumentBuilder() {
        this.documentBuilderFactory.setFeature("disallow-doctype-decl", true);
        this.documentBuilderFactory.setXIncludeAware(false);
        this.documentBuilderFactory.setExpandEntityReferences(false);
        return this.documentBuilderFactory.newDocumentBuilder();
    }

    Object sourceToInputSource(Object source) { return source; }

    Object convertToDocument(Object input) {
        Object object = input;
        if (object instanceof String) {
            return getDocumentBuilder().parse(object);
        }
        else if (object instanceof InputStream) {
            return getDocumentBuilder().parse(object);
        }
        else if (object instanceof byte[]) {
            return getDocumentBuilder().parse(object);
        }
        return getDocumentBuilder().parse(sourceToInputSource(object));
    }
}
`
	counts := runJavaRule(t, rule, "SafeXmlPayloadConverter.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadSpringAIRule(t *testing.T, path string) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent(path)
	if !ok {
		t.Skipf("%s 不在当前构建的 embed FS 中，跳过测试", path)
	}
	require.NotEmpty(t, content, "%s 内容为空", path)
	return content
}

func TestFilterConverterRawStringValueRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-74-improper-neutralization-of-special-elements/java-filter-converter-raw-string-value.sf")
	code := `
abstract class AbstractFilterExpressionConverter {
    protected void doSingleValue(Object value, StringBuilder context) {
        if (value instanceof String) {
            context.append(String.format("\"%s\"", value));
        }
        else {
            context.append(value);
        }
    }
}
`
	counts := runJavaRule(t, rule, "AbstractFilterExpressionConverter.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestFilterConverterRawStringValueRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-74-improper-neutralization-of-special-elements/java-filter-converter-raw-string-value.sf")
	code := `
abstract class AbstractFilterExpressionConverter {
    protected static void emitJsonValue(Object value, StringBuilder context) {
    }

    protected void doSingleValue(Object value, StringBuilder context) {
        emitJsonValue(value, context);
    }
}
`
	counts := runJavaRule(t, rule, "AbstractFilterExpressionConverterSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

func TestMariaDBFilterStringWithoutSQLEscapeRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-89-sql-injection/java-mariadb-filter-string-without-sql-escape.sf")
	code := `
class MariaDBFilterExpressionConverter {
    protected void doSingleValue(Object value, StringBuilder context) {
        if (value instanceof String) {
            context.append(String.format("\'%s\'", value));
        }
        else {
            context.append(value);
        }
    }
}
`
	counts := runJavaRule(t, rule, "MariaDBFilterExpressionConverter.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestMariaDBFilterStringWithoutSQLEscapeRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-89-sql-injection/java-mariadb-filter-string-without-sql-escape.sf")
	code := `
class MariaDBFilterExpressionConverter {
    protected static void emitSqlString(String value, StringBuilder context) {
    }

    protected void doSingleValue(Object value, StringBuilder context) {
        if (value instanceof String stringValue) {
            emitSqlString(stringValue, context);
        }
        else {
            context.append(value);
        }
    }
}
`
	counts := runJavaRule(t, rule, "MariaDBFilterExpressionConverterSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

func TestNeo4jFilterKeyWithoutSanitizeRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-74-improper-neutralization-of-special-elements/java-neo4j-filter-key-without-sanitize.sf")
	code := "class Key {\n" +
		"    String key() { return null; }\n" +
		"}\n\n" +
		"class Neo4jVectorFilterExpressionConverter {\n" +
		"    protected void doKey(Key key, StringBuilder context) {\n" +
		"        context.append(\"node.\").append(\"`metadata.\").append(key.key().replace(\"\\\"\", \"\")).append(\"`\");\n" +
		"    }\n" +
		"}\n"
	counts := runJavaRule(t, rule, "Neo4jVectorFilterExpressionConverter.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestNeo4jFilterKeyWithoutSanitizeRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-74-improper-neutralization-of-special-elements/java-neo4j-filter-key-without-sanitize.sf")
	code := `
class Key {
    String key() { return null; }
}

class SchemaNames {
    static String sanitize(String value, boolean flag) { return value; }
}

class Neo4jVectorFilterExpressionConverter {
    protected void doKey(Key key, StringBuilder context) {
        String sanitized = SchemaNames.sanitize("metadata." + key.key(), true);
        context.append("node.").append(sanitized);
    }
}
`
	counts := runJavaRule(t, rule, "Neo4jVectorFilterExpressionConverterSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

func TestRedisFilterValueWithoutQueryEscapeRule_Positive(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-74-improper-neutralization-of-special-elements/java-redis-filter-value-without-query-escape.sf")
	code := `
import java.util.List;

class Expression {
}

class Value {
    Object value() { return null; }
}

class RedisFilterExpressionConverter {
    private Object stringValue(Expression expression, Value value) {
        String delimiter = " | ";
        if (value.value() instanceof List<?> list) {
            return String.join(delimiter, list.stream().map(String::valueOf).toList());
        }
        return value.value();
    }
}
`
	counts := runJavaRule(t, rule, "RedisFilterExpressionConverter.java", code)
	assert.Greater(t, counts["risk"], 0)
}

func TestRedisFilterValueWithoutQueryEscapeRule_Negative(t *testing.T) {
	rule := loadSpringAIRule(t, "java/cwe-74-improper-neutralization-of-special-elements/java-redis-filter-value-without-query-escape.sf")
	code := `
import java.util.List;

class Expression {
}

class Value {
    Object value() { return null; }
}

class RediSearchUtil {
    static String escapeQuery(String value) { return value; }
}

class RedisFilterExpressionConverter {
    private String escapeTagValue(String value) { return value; }

    private String tagStringValue(Expression expression, Value value) {
        String delimiter = " | ";
        if (value.value() instanceof List<?> list) {
            return list.stream().map(String::valueOf).map(this::escapeTagValue).toList().toString();
        }
        return escapeTagValue(String.valueOf(value.value()));
    }

    private String textStringValue(Expression expression, Value value) {
        if (value.value() instanceof List<?> list) {
            return list.stream().map(String::valueOf).map(RediSearchUtil::escapeQuery).toList().toString();
        }
        return RediSearchUtil.escapeQuery(String.valueOf(value.value()));
    }
}
`
	counts := runJavaRule(t, rule, "RedisFilterExpressionConverterSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts))
}

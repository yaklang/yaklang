package tests

import (
	"embed"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

//go:embed code
var codeFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		_, err := java2ssa.Frontend(src)
		require.Nil(t, err, "parse AST FrontEnd error : %v", err)
	})
}

func legacyEnumIdentifierSource() string {
	return `package com.example;

import java.util.ArrayList;
import java.util.Enumeration;
import java.util.List;
import java.util.Vector;

public class Main {
    public int count(Vector values) {
        int count = 0;
        for (Enumeration enum = values.elements(); enum.hasMoreElements(); ) {
            enum.nextElement();
            count++;
        }
        return count;
    }

    public List<Object> collect(Vector values) {
        List<Object> result = new ArrayList<>();
        Enumeration enum = values.elements();
        if (enum != null) {
            while (enum.hasMoreElements()) {
                result.add(enum.nextElement());
            }
        }
        return result;
    }
}`
}

func TestAllSyntaxForJava_G4(t *testing.T) {
	entry, err := codeFs.ReadDir("code")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		codePath := path.Join("code", f.Name())
		if !strings.HasSuffix(codePath, ".java") {
			continue
		}
		raw, err := codeFs.ReadFile(codePath)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", codePath)
		}
		validateSource(t, codePath, string(raw))
	}
}

func TestCheck1(t *testing.T) {
	badCode := `package org.apache.avalon.framework.logger;

public interface LogEnabled {
	public abstract void enableLogging(Logger var1) {	}
}
`
	validateSource(t, "", badCode)
}

func TestDecompiledParExpressionTrailingComma(t *testing.T) {
	src := `package com.example;

import com.google.gson.reflect.TypeToken;
import java.util.List;

public class Main {
    public Object run(List<String> entities) {
        return helper((new TypeToken<List<String>>() {
        }, ).getType(), (entities != null) ? entities.size() : 0L);
    }
}`
	validateSource(t, "decompiled_par_expression_trailing_comma.java", src)
}

func TestMethodReferenceAndLambdaCollector(t *testing.T) {
	src := `package com.example;

import java.util.Date;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

class TaskBean {
    public String getTaskDefinitionKey() { return ""; }
    public Date getStartTime() { return null; }
}

public class Main {
    public Map<String, TaskBean> run(List<TaskBean> list) {
        return list.stream().collect(Collectors.toMap(TaskBean::getTaskDefinitionKey, t -> t, (o1, o2) -> o1.getStartTime().after(o2.getStartTime()) ? o1 : o2));
    }
}`
	validateSource(t, "method_reference_and_lambda_collector.java", src)
}

func TestDecompiledEmptyCallablePlaceholder(t *testing.T) {
	src := `package com.example;

import java.util.Optional;
import java.util.Set;

public class Main {
    public void run(Set<String> values, Optional<String> value) {
        values.stream().filter(()).map(()).forEach(());
        value.orElseThrow(());
    }
}`
	validateSource(t, "decompiled_empty_callable_placeholder.java", src)
}

func TestDecompiledAnonymousClassMissingComma(t *testing.T) {
	src := `package com.example;

class HttpPost {}
class Entity {}
enum Method { POST; }

abstract class Setting {
    public void headerSet(HttpPost req) {}
}

public class Main {
    public static String execute(Method m, String url, Setting setting, Object entity) {
        return "";
    }

    public static String postHeader(String reqURL) {
        return execute(Method.POST, reqURL, new Setting() {
            public void headerSet(HttpPost req) {}
        }null);
    }

    public static String postJson(String reqURL, Entity stringEntity) {
        return execute(Method.POST, reqURL, new Setting() {
            public void headerSet(HttpPost req) {}
        }(Object)stringEntity);
    }
}`
	validateSource(t, "decompiled_anonymous_class_missing_comma.java", src)
}

func TestBlockFollowedByCastStatement(t *testing.T) {
	src := `package com.example;

class Dao {
    public void update(Object value) {}
}

public class Main {
    private Dao getDao() { return null; }

    public void run(Object value) {
        if (value == null) {
            value = new Object();
        }
        ((Dao)getDao()).update(value);
    }
}`
	validateSource(t, "block_followed_by_cast_statement.java", src)
}

func TestBlockFollowedByParenthesizedExpressionStatement(t *testing.T) {
	src := `package com.example;

import java.util.ArrayList;
import java.util.List;

public class Main {
    public void run(boolean flag) {
        List<String> values = new ArrayList<>();
        values.add("a");
        if (flag) {
            values.add("b");
        }
        (values).add("c");
    }
}`
	validateSource(t, "block_followed_by_parenthesized_expression_statement.java", src)
}

func TestDecompiledNullIdentifierPlaceholder(t *testing.T) {
	src := `package com.example;

import java.util.Iterator;
import java.util.List;

public class Main {
    String sSchema;

    String formatIdentifier(String value) {
        return value;
    }

    String formatName(String value) {
        null = "";
        if (this.sSchema != null && this.sSchema.length() > 0)
            null = this.sSchema + ".";
        return null + formatIdentifier(value);
    }

    void iterate(List<String> values) {
        null = values.iterator();
        while (null.hasNext()) {
            String next = null.next();
        }
    }
}`
	validateSource(t, "decompiled_null_identifier_placeholder.java", src)
}

func TestLegacyEnumIdentifierCompatibility(t *testing.T) {
	src := legacyEnumIdentifierSource()
	validateSource(t, "legacy_enum_identifier.java", src)
	CheckAllJavaCode(src, t)
}

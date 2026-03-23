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

func decompiledSyntheticOuterThisAssignmentSource() string {
	return `package com.example;

public class Outer {
    class Handler {
        private final Outer this$0;

        private Handler(Outer this$0) {
            Outer.this = Outer.this;
        }
    }
}`
}

func decompiledAnonymousClassMissingCommaThisSource() string {
	return `package com.example;

import java.util.Timer;
import java.util.TimerTask;

public class Main {
    long reconnectInterval;

    void run() {
        Timer timer = new Timer("tmc-reconnect", true);
        timer.schedule(new TimerTask() {
            public void run() {}
        }this.reconnectInterval, this.reconnectInterval);
    }
}`
}

func decompiledAnonymousClassMissingCommaNewSource() string {
	return `package com.example;

import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.RejectedExecutionHandler;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;

public class Main {
    void run() {
        ThreadPoolExecutor executor = new ThreadPoolExecutor(10, 100, 60L, TimeUnit.SECONDS, new LinkedBlockingQueue<>(1000), new ThreadFactory() {
            public Thread newThread(Runnable r) {
                return new Thread(r, "xxl-rpc");
            }
        }new RejectedExecutionHandler() {
            public void rejectedExecution(Runnable r, ThreadPoolExecutor executor) {}
        });
    }
}`
}

func decompiledMergeLambdaMissingCommaSource() string {
	return `package com.example;

import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

public class Main {
    public Map<String, List<String>> run() {
        return Arrays.asList("a", "a").stream().collect(Collectors.toMap(v -> v, v -> {
            List<String> list = new java.util.ArrayList<>();
            list.add(v);
            return list;
        }(list1, list2) -> {
            list1.addAll(list2);
            return list1;
        }));
    }
}`
}

func decompiledDuplicateAssignmentTempsSource() string {
	return `package com.example;

import java.util.List;

public class Main {
    public Long run(List<String> values) {
        Long total = Long.valueOf(0L);
        for (String value : values)
            Long long_1 = total, long_2 = total = Long.valueOf(total.longValue() + 1L);
        return total;
    }
}`
}

func decompiledBareCallablePlaceholderSource() string {
	return `package com.example;

import java.util.Arrays;
import java.util.Map;
import java.util.stream.Collectors;

public class Main {
    public Map<String, String> run() {
        return Arrays.asList("a").stream().collect(Collectors.toMap(v -> v, v -> v, ()));
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

func TestDecompiledSyntheticOuterThisAssignment(t *testing.T) {
	src := decompiledSyntheticOuterThisAssignmentSource()
	validateSource(t, "decompiled_synthetic_outer_this_assignment.java", src)
	CheckAllJavaCode(src, t)
}

func TestDecompiledAnonymousClassMissingCommaBeforeThis(t *testing.T) {
	src := decompiledAnonymousClassMissingCommaThisSource()
	validateSource(t, "decompiled_anonymous_class_missing_comma_before_this.java", src)
	CheckAllJavaCode(src, t)
}

func TestDecompiledAnonymousClassMissingCommaBeforeNew(t *testing.T) {
	src := decompiledAnonymousClassMissingCommaNewSource()
	validateSource(t, "decompiled_anonymous_class_missing_comma_before_new.java", src)
	CheckAllJavaCode(src, t)
}

func TestDecompiledMergeLambdaMissingComma(t *testing.T) {
	src := decompiledMergeLambdaMissingCommaSource()
	validateSource(t, "decompiled_merge_lambda_missing_comma.java", src)
	CheckAllJavaCode(src, t)
}

func TestDecompiledDuplicateAssignmentTemps(t *testing.T) {
	src := decompiledDuplicateAssignmentTempsSource()
	validateSource(t, "decompiled_duplicate_assignment_temps.java", src)
	CheckAllJavaCode(src, t)
}

func TestDecompiledBareCallablePlaceholder(t *testing.T) {
	src := decompiledBareCallablePlaceholderSource()
	validateSource(t, "decompiled_bare_callable_placeholder.java", src)
	CheckAllJavaCode(src, t)
}

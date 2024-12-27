package tests

import (
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed interfaceExtends.class
var interfaceExtends []byte

//go:embed enum.class
var enumClass []byte

//go:embed strconv.class
var strconvClass []byte

//go:embed badstrconv.class
var badstrconvClass []byte

//go:embed annotationParam.class
var annotationParam []byte

//go:embed finallydemo.class
var finallydemo []byte

//go:embed tryonly.class
var tryonly []byte

//go:embed synchronizeddemo.class
var synchronizeddemo []byte

//go:embed selfadd.class
var selfadd []byte

//go:embed objectinit.class
var objectinit []byte

//go:embed attribute-demo.class
var attributeDemo []byte

func TestAttributeDemo(t *testing.T) {
	results, err := javaclassparser.Decompile(attributeDemo)
	if err != nil {
		t.Fatal(err)
	}
	/*
		import java.lang.annotation.ElementType;
		import java.lang.annotation.Retention;
		import java.lang.annotation.RetentionPolicy;
		import java.lang.annotation.Target;
		import java.util.List;
		import java.util.ArrayList;
		import java.util.function.Function;

		// 自定义注解定义
		@Retention(RetentionPolicy.CLASS)
		@Target({ElementType.TYPE, ElementType.METHOD, ElementType.FIELD})
		@interface CustomAttribute {
		    String value() default "";
		}

		// 主类
		@CustomAttribute("class-level-attribute")
		public class AttributeDemo<T> {

		    // 内部类
		    private class InnerClass {
		        private void innerMethod() {
		            outerMethod();
		        }
		    }

		    // 泛型字段
		    @CustomAttribute("field-level")
		    private List<T> genericList = new ArrayList<>();

		    // Lambda表达式
		    private Function<String, Integer> lambda = str -> {
		        System.out.println("Converting: " + str);
		        return Integer.parseInt(str);
		    };

		    // 带注解的方法
		    @CustomAttribute("method-level")
		    private void outerMethod() {
		        System.out.println("Outer method called");
		    }

		    // 泛型方法
		    public <E extends Comparable<E>> E genericMethod(E input) {
		        return input;
		    }

		    // 测试方法
		    public static void main(String[] args) {
		        AttributeDemo<String> demo = new AttributeDemo<>();
		        demo.outerMethod();

		        // 测试 Lambda
		        System.out.println(demo.lambda.apply("123"));

		        // 测试泛型方法
		        String result = demo.genericMethod("test");
		        System.out.println(result);
		    }
		}
	*/
	checkJavaCode(t, results)
}

func TestObjectInit(t *testing.T) {
	results, err := javaclassparser.Decompile(objectinit)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
}

/*
try (File o = StaticUtils.OpenFile(...)) {

}

==>

File o = StaticUtils.OpenFile(...);
try {} finally { o.close() }
*/

func TestSelfAdd(t *testing.T) {
	results, err := javaclassparser.Decompile(selfadd)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	assert.Contains(t, results, "++;")
}

func TestSynchronizeddemo(t *testing.T) {
	results, err := javaclassparser.Decompile(synchronizeddemo)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	assert.Contains(t, results, "this.count = (this.count) + (1);")
}

func TestTryCatchFinally(t *testing.T) {
	results, err := javaclassparser.Decompile(finallydemo)
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, results, "System.out.println(\"Finally block")
	checkJavaCode(t, results)
}

func TestTryOnly(t *testing.T) {
	results, err := javaclassparser.Decompile(tryonly)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
}

func TestAnnotationParam(t *testing.T) {
	results, err := javaclassparser.Decompile(annotationParam)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	assert.Contains(t, results, "phase=")
	assert.Contains(t, results, `ndicator(phase=ProcessingPhase.DEPENDENCY_ANALY`)
	assert.Contains(t, results, `import ch.qos.logback.core.model.processor.ProcessingPhase;`)
}

func TestStrconv2(t *testing.T) {
	results, err := javaclassparser.Decompile(badstrconvClass)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	assert.Contains(t, results, `ement [" + this.tag + "] near line " + Action.getLineNumber(this.intercon));`)
}

func TestStrconv(t *testing.T) {
	results, err := javaclassparser.Decompile(strconvClass)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	assert.Contains(t, results, `he value \"\" is not a legal value for attribute \""`)
}

func TestEnumBasic(t *testing.T) {
	results, err := javaclassparser.Decompile(enumClass)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	fmt.Println(results)
	assert.Contains(t, results, "enum Node$Type")
	assert.Contains(t, results, "\tLITERAL,\n\tVARIABLE;\n")
}

func TestInterfaceExtends(t *testing.T) {
	results, err := javaclassparser.Decompile(interfaceExtends)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	fmt.Println(results)
	assert.Contains(t, results, "NavigableSet extends SortedSet")
}

// checkjavacode
func checkJavaCode(t *testing.T, code string) {
	fmt.Println(code)
	ssatest.CheckJava(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	})
	fmt.Println(code)
}

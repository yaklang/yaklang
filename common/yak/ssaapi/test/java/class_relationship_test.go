package java

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

const javaClassRelationShipCode = `// 定义一个接口
interface MyInterface extends ABC {
    void doSomething();
}

// 定义一个抽象类
abstract class MyAbstractClass {
    abstract void doAbstractThing();
}

// 定义一个普通的父类
class MyParentClass {
    void doParentThing() {
        System.out.println("Doing parent thing");
    }
}

// 定义一个类，这个类继承了 MyParentClass，并实现了 MyInterface
public class MyClass extends MyParentClass implements MyInterface {
    // 实现 MyInterface 的方法
    @Override
    public void doSomething() {
        System.out.println("Doing something");
    }

    // 定义一个内部类
    class MyInnerClass {
        void doInnerThing() {
            System.out.println("Doing inner thing");
        }
    }

    // 定义一个静态内部类
    static class MyStaticInnerClass {
        static void doStaticInnerThing() {
            System.out.println("Doing static inner thing");
        }
    }

    // 继承并实现 MyAbstractClass 的抽象方法
    class MyConcreteClass extends MyAbstractClass {
        @Override
        void doAbstractThing() {
            System.out.println("Doing abstract thing");
        }
    }
}
`

func TestJavaClassRelationship(t *testing.T) {
	ssatest.CheckJava(t, javaClassRelationShipCode, func(prog *ssaapi.Program) error {
		var ret ssaapi.Values
		ret = prog.SyntaxFlowChain(`MyClass_declare?{.__parents__?{have: MyParentClass || have: MyInterface}}`).Show()
		assert.Equal(t, 1, len(ret))

		ret = prog.SyntaxFlowChain(`MyClass?{.__interface__?{!have: MyParentClass && have: MyInterface}}`).Show()
		assert.Equal(t, 1, len(ret))
		return nil
	})
}

func TestJavaClassRelationship_2(t *testing.T) {
	ssatest.CheckJava(t, javaClassRelationShipCode, func(prog *ssaapi.Program) error {
		var ret ssaapi.Values
		ret = prog.SyntaxFlowChain(`.__parents__?{have: ABC}<getObject>`).Show()
		assert.Equal(t, 1, len(ret))
		ret = prog.SyntaxFlowChain(`*declare?{.__parents__?{have: Parent}}?{.__parents__?{have:MyInterface}} `).Show()
		assert.Equal(t, 1, len(ret))
		return nil
	})
}

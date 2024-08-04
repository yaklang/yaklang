package java

import (
	"github.com/stretchr/testify/assert"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const NativeCallTest = `

@RestController(value = "/xxe")
public class XXEController {
    @RequestMapping(value = "/one")
    public String yourMethod(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        DocumentBuilder documentBuilder = DocumentBuilderFactory.newInstance().newDocumentBuilder();
        InputStream stream = new ByteArrayInputStream(xmlStr.getBytes("UTF-8"));
        org.w3c.dom.Document doc = documentBuilder.parse(stream);
        doc.getDocumentElement().normalize();
        return "Hello World";
    }

    public String HHHHH(@RequestParam(value = "xxx") String xxxFooBar) throws Exception {
        return "Hello getReturns";
    }
}


public class Demo2 {
	@AutoWired
	XXEController xxeController = null;

    public String one() throws Exception {
        xxeController.yourMethod("Hello Native Method");
    }
}

public class Demo3 {
	@AutoWired
	XXEController xxeController = null;

    public String one() throws Exception {
		var aArgs = new String[]{"aaaaaaa"};
        xxeController.yourMethod(aArgs);
    }
}

public class Demo4 {
	
	AnothorController controller = null;

    public String one() throws Exception {
		var flexible = new String[]{"bbbbbb"};
        controller.yourMethod(flexible);
    }
}

`

func TestNativeCall_GetObject(t *testing.T) {
	ssatest.Check(t, `a = {"b": 111, "c": 222, "e": 333}`,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
.b<getObject>.c as $sink;
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			if !strings.Contains(sink.String(), "222") {
				t.Fatal("sink[0].String() != 222")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.Yak),
	)
}

func TestNativeCall_GetReturns(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
HHHHH <getReturns> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			if !strings.Contains(sink.String(), "Hello getReturns") {
				t.Fatal("sink[0].String() != Hello getReturns")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetFormalParams(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
HHHHH <getFormalParams> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 2 {
				t.Fatal("sink.Len() != 2")
			}

			if !utils.MatchAllOfSubString(sink.String(), "xxxFooBar", "this") {
				t.Fatal("sink[0].String() !contains xxxFooBar / this")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_SearchCall(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
flexible <getCall> <searchFunc> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			if sink.Len() <= 1 {
				t.Fatal("sink.Len() <= 1")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetCall(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
aArgs <getCall> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetCall_Then_GetFunc(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
yourMethod()<getCaller> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			sink.Show()
			if sink.Len() < 2 {
				t.Fatal("sink.Len() != 1")
			}
			for _, val := range sink {
				if !strings.Contains(val.String(), "yourMethod") {
					t.Fatal("sink[0].GetName() != yourMethod")
				}
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetCaller(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
aArgs <getCall> <getCaller> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			sink[0].Show()

			if !strings.Contains(sink[0].String(), "yourMethod") {
				t.Fatal("sink[0].String() != yourMethod")
			}

			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetFunc(t *testing.T) {
	ssatest.Check(t, `

yourMethod = () => {
	c(aArgs);
}

`,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
aArgs <getFunc> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			sink[0].Show()

			if !strings.HasSuffix(sink[0].String(), "yourMethod") {
				t.Fatal("sink[0].String() != yourMethod")
			}

			return nil
		},
		ssaapi.WithLanguage(ssaapi.Yak),
	)
}

func TestNativeCall_SearchFormalParams(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			prog.Show()
			results := prog.SyntaxFlow("DocumentBuilderFactory...parse(* #-> as $source) as $sink", sfvm.WithEnableDebug(true))
			results.Show()
			ssatest.CompareResult(t, true, results, map[string][]string{
				"source": {`"Hello Native Method"`, `"aaaaaaa"`},
			})
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_FuncName(t *testing.T) {
	ssatest.Check(t, `
funcA = () => {
	return "abc";
}
`, func(prog *ssaapi.Program) error {
		sinks := prog.SyntaxFlowChain(`funcA<name> as $sink`).Show()
		haveFuncA := false
		for _, v := range sinks {
			if strings.Contains(v.String(), "funcA") {
				haveFuncA = true
			}
		}
		assert.True(t, haveFuncA)
		return nil
	})
}

func TestNativeCall_Java_FuncName(t *testing.T) {
	ssatest.Check(t, NativeCallTest, func(prog *ssaapi.Program) error {
		sinks := prog.SyntaxFlowChain(`aArgs<getCall><getCaller><name> as $sink`).Show()
		haveFuncName := false
		for _, v := range sinks {
			if strings.Contains(v.String(), "yourMethod") {
				haveFuncName = true
			}
		}
		assert.True(t, haveFuncName)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestNativeCall_Java_Eval(t *testing.T) {
	ssatest.Check(t, NativeCallTest, func(prog *ssaapi.Program) error {
		sinks := prog.SyntaxFlowChain(`
<eval('aArgs<getCall><getCaller><name> as $sink')>
`).Show()
		haveFuncName := false
		for _, v := range sinks {
			if strings.Contains(v.String(), "yourMethod") {
				haveFuncName = true
			}
		}
		assert.True(t, haveFuncName)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestNativeCall_Java_Eval_Show(t *testing.T) {
	ssatest.Check(t, NativeCallTest, func(prog *ssaapi.Program) error {
		sinks := prog.SyntaxFlowChain(`
<eval('aArgs<getCall><getCaller><show><name> as $sink')>
`).Show()
		haveFuncName := false
		for _, v := range sinks {
			if strings.Contains(v.String(), "yourMethod") {
				haveFuncName = true
			}
		}
		assert.True(t, haveFuncName)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestNativeCall_Java_FuzztagNEval(t *testing.T) {
	ssatest.Check(t, NativeCallTest, func(prog *ssaapi.Program) error {
		sinks := prog.SyntaxFlowChain(`
<fuzztag("<getCaller>")> as $accccc;
<fuzztag('aArgs<getCall>{{accccc}}<name> as $sink')> as $code;
<eval($code)><show>
check $sink;
`).Show()
		haveFuncName := false
		for _, v := range sinks {
			if strings.Contains(v.String(), "yourMethod") {
				haveFuncName = true
			}
		}
		assert.True(t, haveFuncName)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestNativeCall_Java_FuzztagThenEval_Basic(t *testing.T) {
	ssatest.Check(t, NativeCallTest, func(prog *ssaapi.Program) error {
		sinks := prog.SyntaxFlowChain(`
<fuzztag("<getCaller>")> as $accccc;
<fuzztag('aArgs<getCall>{{accccc}}<name> as $sink')> as $code;
<eval($code)><show>
check $sink;
`).Show()
		haveFuncName := false
		for _, v := range sinks {
			if strings.Contains(v.String(), "yourMethod") {
				haveFuncName = true
			}
		}
		assert.True(t, haveFuncName)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestNativeCall_Java_FuzztagThenEval(t *testing.T) {
	ssatest.Check(t, `a1=1;a2=2;a3=3;`, func(prog *ssaapi.Program) error {
		sinks := prog.SyntaxFlowChain(`
<fuzztag('a{{int(1-3)}} as $sink')><eval><show>;
check $sink;
`).Show()
		assert.Len(t, sinks, 3)
		return nil
	})
}

const mybatisTest = `
import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import org.apache.ibatis.annotations.*;
import java.util.List;

public interface UserMapper extends BaseMapper<User> {
    @Select("SELECT * FROM users WHERE age = #{age} AND name = #{name} AND email = #{email}")
    List<User> selectUsersByMultipleFields(int age, String name, String email);

    @Select("SELECT * FROM ${tableName} WHERE age = #{age}")
    List<User> selectUsersByTableName(String tableName, int age);

    @Delete("DELETE FROM users WHERE id = #{id}")
    int deleteUserById(Long id);

    @Update("UPDATE users SET email = #{email} WHERE id = #{id}")
    int updateUserEmailById(Long id, String email);

    @Insert("INSERT INTO users (name, age, email) VALUES (#{name}, #{age}, ${email})")
    int insertUser(String name, int age, String email);

    @Select("SELECT * FROM users WHERE email LIKE CONCAT('%', #{email}, '%')")
    List<User> findUsersByEmail(String email);

    // 动态 SQL 使用例子
    @Select("<script>" +
            "SELECT * FROM users " +
            "<where> " +
            "   <if test='name != null'> AND name = #{name} </if>" +
            "   <if test='email != null'> AND email = #{email} </if>" +
            "</where>" +
            "</script>")
    List<User> findUsersByOptionalCriteria(@Param("name") String name, @Param("email") String email);

    // 批量删除
    @Delete("<script>" +
            "DELETE FROM users WHERE id IN " +
            "<foreach item='id' collection='ids' open='(' separator=',' close=')'>" +
            "   #{id}" +
            "</foreach>" +
            "</script>")
    int deleteUsersByIds(@Param("ids") List<Long> ids);

    // 更新多个字段
    @Update("UPDATE users SET age = #{age}, email = #{email} WHERE id = #{id}")
    int updateUserById(Long id, int age, String email);
}
`

func TestNativeCall_Java_RegexpForMybatisAnnotation(t *testing.T) {
	ssatest.CheckJava(t, mybatisTest, func(prog *ssaapi.Program) error {
		var results ssaapi.Values
		var checked bool
		results = prog.SyntaxFlowChain(`

.annotation.Select.value<show><regexp(` + strconv.Quote(`\$\{\s*(\w+)\s*\}`) + `,group=1)> as $entry;

`).Show()
		checked = false
		if results.Len() >= 1 {
			results.Recursive(func(operator sfvm.ValueOperator) error {
				if strings.Contains(operator.String(), "tableName") {
					checked = true
					return nil
				}
				return nil
			})
		}
		if !checked {
			t.Fatal("not found tableName")
		}

		results = prog.SyntaxFlowChain(`

.annotation.Insert.value<show><regexp(` + strconv.Quote(`\$\{\s*(\w+)\s*\}`) + `,group=1)> as $entry;

`).Show()
		checked = false
		if results.Len() >= 1 {
			results.Recursive(func(operator sfvm.ValueOperator) error {
				if strings.Contains(operator.String(), "email") {
					checked = true
					return nil
				}
				return nil
			})
		}
		if !checked {
			t.Fatal("not found tableName")
		}
		return nil
	})
}

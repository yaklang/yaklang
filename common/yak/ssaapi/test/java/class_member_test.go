package java

import (
	"fmt"
	"testing"
)

func Test_CrossClass_SideEffect_Exec_Case(t *testing.T) {
	tests := []struct {
		name   string
		equal  bool
		expect []string
		code   string
	}{
		{"aTaintCase022", false, []string{"Parameter-cmd", "Undefined-Runtime"},
			`/**
   * 字段/元素级别->对象字段->对象元素
   * case应该被检出
   */
  @PostMapping(value = "case022")
  public Map<String, Object> aTaintCase022(@RequestParam String cmd) {
      Map<String, Object> modelMap = new HashMap<>();
      try {
          CmdObject simpleBean = new CmdObject();
          simpleBean.setCmd(cmd);
          simpleBean.setCmd2("cd /");
          Runtime.getRuntime().exec(simpleBean.getCmd());
          modelMap.put("status", "success");
      } catch (Exception e) {
          modelMap.put("status", "error");
      }
      return modelMap;
  }`},
		{"aTaintCase022_2", true, []string{"\"cd /\"", "Undefined-Runtime"}, ` /**
		  * 字段/元素级别->对象字段->对象元素
		  * case不应被检出
		  */
		 @PostMapping(value = "case022-2")
		 public Map<String, Object> aTaintCase022_2(@RequestParam String cmd) {
		     Map<String, Object> modelMap = new HashMap<>();
		     try {
		         CmdObject simpleBean = new CmdObject();
		         simpleBean.setCmd(cmd);
		         simpleBean.setCmd2("cd /");
		         Runtime.getRuntime().exec(simpleBean.getCmd2());
		         modelMap.put("status", "success");
		     } catch (Exception e) {
		         modelMap.put("status", "error");
		     }
		     return modelMap;
		 }
		`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.code = createCmdObject(tt.code)
			testExecTopDef(t, &TestCase{
				Code:    tt.code,
				Contain: !tt.equal,
				Expect: map[string][]string{
					"target": tt.expect,
				},
			})
		})
	}
}

func createCmdObject(code string) string {
	CmdUtilCode := fmt.Sprintf(`package com.sast.astbenchmark.model;

public class CmdObject {
    private String cmd1;
    private String cmd2;

    public void setCmd(String s) {
        this.cmd1 = s;
    }

    public void setCmd2(String s) {
        this.cmd2 = s;
    }

    public String getCmd() {
        return this.cmd1;
    }

    public String getCmd2() {
        return this.cmd2;
    }
}
@RestController()
public class AstTaintCase001 {
%v
}`, code)
	return CmdUtilCode
}

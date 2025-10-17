package java

import (
	"fmt"
	"testing"
)

type TestCaseSimple struct {
	name   string
	equal  bool
	expect []string
	code   string
}

var tests = []TestCaseSimple{
	{"aTaintCase011", true, []string{"Parameter-cmd", "Undefined-Runtime"}, `@GetMapping("case011/{cmd}")
    public Map<String, Object> aTaintCase011(@PathVariable String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String a = cmd;
            Runtime.getRuntime().exec(a);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase012", true, []string{"Parameter-cmd", "Undefined-Runtime"}, `@GetMapping("case011/{cmd}")
    public Map<String, Object> aTaintCase011(@PathVariable String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String a = cmd;
            Runtime.getRuntime().exec(a);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0110",
		false, []string{
			`Parameter-request`,
			`"cmd"`,
		},
		` @PostMapping("case0110")
            public Map<String, Object> aTaintCase0110(@RequestParam("cmd") String cmd, HttpServletRequest request) {
                    String cmdStr = request.getParameterMap().get("cmd")[0];
                    Runtime.getRuntime().exec(cmdStr);
            
            }`,
	},
	{"aTaintCase0111", false,
		[]string{
			"Parameter-request",
		},
		` /**
     * Arrayaccess
     * @param request
     * @return
     */
    @PostMapping(value = "case0111")
    public Map<String, Object> aTaintCase0111( HttpServletRequest request) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Cookie[] cookies = request.getCookies();
            Runtime.getRuntime().exec(cookies[0].getName());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
    `},
	{"aTaintCase0113", false, []string{"\" &\"", "Parameter-cmd", "Undefined-Runtime", "Undefined-String", "Undefined-String"},
		`/**
     * classinstance + initfix
     */
    @PostMapping(value = "case0113")
    public Map<String, Object> aTaintCase0113(@RequestParam(defaultValue = "ls") String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(new String(cmd+" &"));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0117", true, []string{"Parameter-cmd", "Undefined-Runtime"}, ` /**
		* arrayaccess
		*/
		@PostMapping(value = "case0117")
		public Map<String, Object> aTaintCase0117(@RequestParam String cmd) {
		   Map<String, Object> modelMap = new HashMap<>();
		   try {
		       String[] strings = new String[3];
		       strings[0]="cd ~";
		       strings[1]=cmd;
		       strings[2]="cd /";
		       Runtime.getRuntime().exec(strings[1]);
		       modelMap.put("status", "success");
		   } catch (Exception e) {
		       modelMap.put("status", "error");
		   }
		   return modelMap;
		}`},
	{"aTaintCase0118", true, []string{"\" \"", "\"mkdir\"", "Parameter-cmd", "Undefined-Runtime"},
		`    /**
     * WhileStatement
     * @param cmd
     * @param type
     * @return
     */

    @GetMapping("case0118/{type}/{cmd}")
    public Map<String, Object> aTaintCase0118(@PathVariable String cmd,@PathVariable String type) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String a ="mkdir";;
            while(StringUtils.equals(type,"mkdir")) {
                a = " "+ cmd;
            }
            Runtime.getRuntime().exec(a);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0127", true, []string{"\"mkdir\"", "\"|\"", "Parameter-cmd", "Undefined-Runtime"},
		` /**
     * forstatement
     * @param cmd
     * @return
     */
    @GetMapping("case0127/{cmd}")
    public Map<String, Object> aTaintCase0127(@PathVariable String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String a ="mkdir";
            for(int i =0 ;i<10; i++){
                a= cmd+"|";
            }
            Runtime.getRuntime().exec(a);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0128", true, []string{"\"mkdir\"", "\"|\"", "Parameter-cmd", "Undefined-Runtime"},
		`/**
     * DoStatement
     * @param cmd
     * @return
     */
    @GetMapping("case0128/{cmd}")
    public Map<String, Object> aTaintCase0128(@PathVariable String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String a ="mkdir";
            int i = 10;

            do {
                a= cmd+"|";
                i++;
            }while (i<20);

            Runtime.getRuntime().exec(a);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0129", true, []string{"Parameter-cmd", "Undefined-Runtime"}, `   /**
     * CastExpression
     * @param cmd
     * @return
     */
    @GetMapping("case0129/{cmd}")
    public Map<String, Object> aTaintCase0129(@PathVariable String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Object cmdObject = new Object();
            cmdObject=cmd;
            Runtime.getRuntime().exec((String) cmdObject);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0134", false, []string{
		"Parameter-cmd",
		"Parameter-methodname",
	},
		`/**
		* 反射调用
		* @param cmd
		* @return
		*/
		@GetMapping("case0134/{cmd}/{methodname}")
		public Map<String, Object> aTaintCase0134(@PathVariable String cmd,@PathVariable String methodname) {
		   Map<String, Object> modelMap = new HashMap<>();
		   if (cmd == null) {
		       modelMap.put("status", "error");
		       return modelMap;
		   }
		   try {
		       Class<CmdUtil> clazz = CmdUtil.class;
		       Method method = clazz.getMethod(methodname, String.class);
		       method.setAccessible(true);
		       cmd = (String) method.invoke(clazz.newInstance(), cmd);
		       Runtime.getRuntime().exec(cmd);
		       modelMap.put("status", "success");
		   } catch (Exception e) {
		       modelMap.put("status", "error");
		   }
		   return modelMap;
		}`},
	{"aTaintCase0135", false, []string{
		"Parameter-cmd", "1",
	}, `
    /**
     * PrefixExpression
     */
    @GetMapping("case0135/{cmd}")
    public Map<String, Object> aTaintCase0135(@PathVariable int cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd++;
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0136", false, []string{"Parameter-cmd", "1"}, ` /**
     * PostfixExpression
     */
    @GetMapping("case0136/{cmd}")
    public Map<String, Object> aTaintCase0136(@PathVariable int cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            ++cmd;
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0137", false, []string{"Parameter-cmd", "Undefined-Runtime", "Undefined-String"},
		`/**
* 基本类型char 作为污点源
     * 测试数据传（0～9）
     * @return
     */
    @GetMapping("case0137/{cmd}")
    public Map<String, Object> aTaintCase0137(@PathVariable char cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0138", false, []string{"Parameter-cmd", "Undefined-Runtime", "Undefined-String"}, `  /**
     * 基本类型byte 作为污点源
     * @param cmd
     * @return
     */
    @GetMapping("case0138/{cmd}")
    public Map<String, Object> aTaintCase0138(@PathVariable byte cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0139", false, []string{"Parameter-cmd", "Undefined-Runtime"},
		`/**
     * 基础类型long 作为污点源
     *
     * @param cmd
     * @return
     */
    @GetMapping("case0139/{cmd}")
    public Map<String, Object> aTaintCase0139(@PathVariable long cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0140", false, []string{"Parameter-cmd", "Undefined-Runtime"}, `  /**
     * 引用类型Map 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0140")
    public Map<String, Object> aTaintCase0140(@RequestBody Map<String, String> cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null || cmd.isEmpty()) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(cmd.toString());
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0141", false, []string{"Parameter-cmd", "Undefined-Runtime"}, `
    /**
     * 引用类型List 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0141")
    public Map<String, Object> aTaintCase0141(@RequestBody List<String> cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null || CollectionUtils.isEmpty(cmd)) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(cmd.get(0));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0144", false, []string{"Parameter-cmd", "Undefined-Runtime"}, ` /**
     * 基本数据类型的封装类型 Byte 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0144/{cmd}")
    public Map<String, Object> aTaintCase0144(@PathVariable Byte cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(cmd.toString());
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0145", false, []string{"Parameter-cmd", "Undefined-Runtime"},
		`/**
     * 基本数据类型的封装类型 Integer 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0145/{cmd}")
    public Map<String, Object> aTaintCase0145(@PathVariable Integer cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0146", false, []string{"Parameter-cmd", "Undefined-Runtime"}, `
    /**
     * 基本数据类型的封装类型 Long 作为污点源
     * @param cmd
     * @return
     */
    @PostMapping("case0146/{cmd}")
    public Map<String, Object> aTaintCase0146(@PathVariable Long cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0147", false, []string{"Parameter-cmd", "Undefined-Runtime"}, ` /**
     * 基本数据类型的封装类型 Character 作为污点源
     * @param cmd 测试数据使用（0~9）
     * @return
     */
    @PostMapping("case0147/{cmd}")
    public Map<String, Object> aTaintCase0148(@PathVariable Character cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0149", false, []string{"Parameter-cmd", "Undefined-Runtime"},
		`/**
     * 数组 String[] 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0149")
    public Map<String, Object> aTaintCase0149(@RequestBody String[] cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null || cmd.length < 1) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(cmd[0]);
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`}, //数组 String[] 作为污点源
	{"aTaintCase0150", false, []string{"Undefined-Runtime", "Parameter-cmd", "2"},
		`/**
			 * 数组 char[] 作为污点源
			 *
			 * @param cmd [1,2]
			 * @return
			 */
			@PostMapping("case0150")
			public Map<String, Object> aTaintCase0150(@RequestBody int[] cmd) {
			Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null || cmd.length < 1) {
            modelMap.put("status", "error");
            return modelMap;
        }
        char[] data = {(char) cmd[0], 2};
        try {
            Runtime.getRuntime().exec(data.toString());
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`}, //数组 char[] 作为污点源
	{"aTaintCase0151", false, []string{"Parameter-cmd", "Undefined-Runtime"}, ` /**
     * 数组 byte[] 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0151")
    public Map<String, Object> aTaintCase0151(@RequestBody byte[] cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null || cmd.length < 1) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(cmd.toString());
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0152", true, []string{"Parameter-cmd", "Undefined-Runtime", "nil"}, `    /**
     * 其他对象 String 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0152")
    public Map<String, Object> aTaintCase0152(@RequestBody String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0153", true, []string{"Parameter-cmd", "Undefined-Runtime", "nil"}, `    /**
     * 其他对象 String 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0152")
    public Map<String, Object> aTaintCase0152(@RequestBody String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null) {
            modelMap.put("status", "error");
            return modelMap;
        }
        try {
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},

	{"aTaintCase0155", false, []string{"Parameter-cmd", "Undefined-Runtime"}, ` /**
     * 类对象找不到对应的实现类
     *
     * @param
     * @return
     */
    @PostMapping(value = "case0155")
    public Map<String, Object> aTaintCase0155(@RequestParam(defaultValue = "ls") String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String exec = String.valueOf(cmd);
            Runtime.getRuntime().exec(exec);
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
	{"aTaintCase0158", true, []string{"\"ls\"", "Undefined-Runtime"},
		`/**
     * 传播场景
     */
    /**
     * 传播场景->运算符->赋值
     */
    @PostMapping(value = "case0158")
    public Map<String, Object> aTaintCase0158(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd= "ls";
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0159", false, []string{"Parameter-cmd", "1"},
		`/**
     * 61 传播场景->运算符->位运算
     */
    @PostMapping(value = "case0159")
    public Map<String, Object> aTaintCase0159(@RequestParam char cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd= (char) (cmd<<1);
            Runtime.getRuntime().exec(String.valueOf(cmd));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	//传播场景->String操作
	{"aTaintCase0161", false, []string{"Parameter-cmd", `" -la"`}, `  /**
     * 63 传播场景->String操作->conact
     */
    @PostMapping(value = "case0161")
    public Map<String, Object> aTaintCase0161(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.concat(" -la");
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0162", false, []string{"Parameter-cmd", "Undefined-Runtime"}, `  /**
     * 传播场景->String操作->copyValueOf
     */
    @PostMapping(value = "case0162")
    public Map<String, Object> aTaintCase0162(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            char data[] =  cmd.toCharArray();
            Runtime.getRuntime().exec(String.copyValueOf(data));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0163", false, []string{`"%s -la"`, "Parameter-cmd"}, ` /**
		* 65 传播场景->String操作->format
		*/
		@PostMapping(value = "case0163")
		public Map<String, Object> aTaintCase0163(@RequestParam String cmd ) {
		   Map<String, Object> modelMap = new HashMap<>();
		   try {
		       cmd = String.format("%s -la",cmd);
		       Runtime.getRuntime().exec(cmd);
		       modelMap.put("status", "success");
		   } catch (Exception e) {
		       modelMap.put("status", "error");
		   }
		   return modelMap;
		}`},
	{"aTaintCase0164", false, []string{"Parameter-cmd", "Undefined-Runtime"},
		`/**
     * 66 传播场景->String操作->getBytes
     */

    @PostMapping(value = "case0164")
    public Map<String, Object> aTaintCase0164(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            byte[] bytes = cmd.getBytes();
            Runtime.getRuntime().exec(String.valueOf(bytes));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},

	{"aTaintCase0166", false, []string{"Parameter-cmd", "Undefined-Runtime"},
		`/**
     * 68 传播场景->String操作->intern
     */
    @PostMapping(value = "case0166")
    public Map<String, Object> aTaintCase0166(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(cmd.intern());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	{"aTaintCase0167", false, []string{"Parameter-cmd", `" "`, `"-la"`, "Undefined-Runtime"},
		`/**
		   * 69 传播场景->String操作->join
		   */
		  @PostMapping(value = "case0167")
		  public Map<String, Object> aTaintCase0167(@RequestParam String cmd ) {
		      Map<String, Object> modelMap = new HashMap<>();
		      try {
		          cmd=String.join(" ",cmd,"-la");
		          Runtime.getRuntime().exec(cmd);
		          modelMap.put("status", "success");
		      } catch (Exception e) {
		          modelMap.put("status", "error");
		      }
		      return modelMap;
		  }
		`},

	{"aTaintCase0177", false, []string{"Parameter-cmd", "Undefined-Runtime"}, ` /**
     * 78 传播场景->String操作->toString
     */
    @PostMapping(value = "case0177")
    public Map<String, Object> aTaintCase0177(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.toString();
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},

	{"aTaintCase0195", false, []string{
		"Parameter-cmd",
		// "ParameterMember-parameter[1].0",
		// "ParameterMember-parameter[1].1",
	}, ` /**
     * 传播场景-数组初始化->new 方式初始化
     */
    @PostMapping(value = "case0195")
    public Map<String, Object> aTaintCase0195(@RequestParam String[] cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String[] chars = new String[]{cmd[0],cmd[1]};
            Runtime.getRuntime().exec(chars);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
}

func run(t *testing.T, tt TestCaseSimple) {
	t.Run(tt.name, func(t *testing.T) {
		allCode := fmt.Sprintf(`
            package com.sast.astbenchmark.cases;
            @RestController()
            public class AstTaintCase{
                private SSRFShowManager ssrfShowManager = new SSRFShowManageImpl();
                %v
            }`, tt.code)
		testExecTopDef(t, &TestCase{
			Code: allCode,
			Expect: map[string][]string{
				"target": tt.expect,
			},
			Contain: !tt.equal,
		})
	})
}

func Test_Simple_Exec_Case(t *testing.T) {
	for _, tt := range tests {
		run(t, tt)
	}
}

func Test_Simple_Exec_Case_Debug(t *testing.T) {
	target := "aTaintCase0150"

	for _, tt := range tests {
		if tt.name != target {
			continue
		}
		run(t, tt)
	}
}

// TODO：待完善java官方库的方法
//func Test_Special_Exec_Case(t *testing.T) {
//	tests := []struct {
//		name   string
//		target string
//		code   string
//	}{
//		{"aTaintCase018",true, []string{"Parameter-cmd", "Undefined-Runtime",}, `@PostMapping("case018/{cmd}")
//   public Map<String, Object> aTaintCase018(@PathVariable String cmd) {
//       Map<String, Object> modelMap = new HashMap<>();
//       if (cmd == null) {
//           modelMap.put("status", "error");
//           return modelMap;
//       }
//       try {
//           String[] b = {"a","b"};
//           System.arraycopy(cmd,0,b,0,2);
//           Runtime.getRuntime().exec(b[0]);
//           modelMap.put("status", "success");
//       } catch (Exception e) {
//           modelMap.put("status", "error");
//       }
//       return modelMap;
//   }
//`}, //System.arraycopy方法
//		{"aTaintCase019",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` @PostMapping("case019")
//    public Map<String, Object> aTaintCase019(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        char[] data = cmd.toCharArray();
//        try {
//            Runtime.getRuntime().exec(new String(data));
//            modelMap.put("status", "success");
//        } catch (IOException e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`}, // 实例化一个不存在的类
//		{"aTaintCase0114",true, []string{"Parameter-cmd", "Undefined-Runtime",}, `  /**
//     * MI+MI
//     * @param cmd
//     * @return
//     */
//    @PostMapping(value = "case0114")
//    public Map<String, Object> aTaintCase0114(@RequestParam(defaultValue = "ls") String cmd ) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd.toUpperCase());
//            Runtime.getRuntime().exec(builder.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`}, //StringBuilder
//		{"aTaintCase0115",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * MI+arguement
//     */
//    @PostMapping(value = "case0115")
//    public Map<String, Object> aTaintCase0115(@RequestParam String cmd ) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            char[] chars= new char[]{0,0};
//            cmd.getChars(0,2,chars,0);
//            Runtime.getRuntime().exec(String.valueOf(chars));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`}, //getChars
//		{"aTaintCase0142",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 引用类型queue 作为污点源
//     *
//     * @param cmd
//     * @return
//     */
//    @PostMapping("case0142")
//    public Map<String, Object> aTaintCase0142(@RequestBody List<String> cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        if (cmd == null || CollectionUtils.isEmpty(cmd)) {
//            modelMap.put("status", "error");
//            return modelMap;
//        }
//        Queue<String> queue = new LinkedBlockingQueue();
//        try {
//            queue.add(cmd.get(0));
//            Runtime.getRuntime().exec(queue.peek());
//            modelMap.put("status", "success");
//        } catch (IOException e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`}, //引用类型queue 作为污点源
//		{"aTaintCase0143",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` /**
//     * 引用类型Set 作为污点源
//     *
//     * @param cmd
//     * @return
//     */
//    @PostMapping("case0143")
//    public Map<String, Object> aTaintCase0143(@RequestBody List<String> cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        if (cmd == null || CollectionUtils.isEmpty(cmd)) {
//            modelMap.put("status", "error");
//            return modelMap;
//        }
//        Set<String> stringSet = new HashSet<>(cmd);
//        try {
//
//            Runtime.getRuntime().exec(stringSet.stream().iterator().next());
//            modelMap.put("status", "success");
//        } catch (IOException e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }
//`}, //引用类型Set 作为污点源
//		{"aTaintCase0154",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 其他对象 StringBuilder 作为污点源
//     *
//     * @param cmd
//     * @return
//     */
//    @PostMapping("case0154")
//    public Map<String, Object> aTaintCase0154(@RequestBody String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        if (cmd == null) {
//            modelMap.put("status", "error");
//            return modelMap;
//        }
//        StringBuilder data = new StringBuilder();
//        data.append(cmd);
//        try {
//            Runtime.getRuntime().exec(data.toString());
//            modelMap.put("status", "success");
//        } catch (IOException e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }
//
//`}, // StringBuilder 作为污点源
//		{"aTaintCase0160",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` /**
//     * 62 传播场景->String操作->构造方法
//     */
//    @PostMapping(value = "case0160")
//    public Map<String, Object> aTaintCase0160(@RequestParam String cmd ) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            Runtime.getRuntime().exec(new String(cmd));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0165",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 67 传播场景->String操作->getChars
//     */
//    @PostMapping(value = "case0165")
//    public Map<String, Object> aTaintCase0165(@RequestParam String cmd ) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            char[] chars= new char[]{0,0};
//            cmd.getChars(0,2,chars,0);
//            Runtime.getRuntime().exec(String.valueOf(chars));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0169",true, []string{"Parameter-cmd", "Undefined-Runtime",}, `  /**
//     * 71 传播场景->String操作->replace
//     * ls;-la
//     */
//    @PostMapping(value = "case0169")
//    public Map<String, Object> aTaintCase0169(@RequestParam String cmd ) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            cmd=cmd.replace(";"," ");
//            Runtime.getRuntime().exec(cmd);
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }
//`},
//		{"aTaintCase0170",true, []string{"Parameter-cmd", "Undefined-Runtime",}, `    /**
//     *  传播场景->String操作->replace
//     * alasa
//     */
//    @PostMapping(value = "case0170")
//    public Map<String, Object> aTaintCase0170(@RequestParam String cmd ) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            cmd=cmd.replaceAll("a","");
//            Runtime.getRuntime().exec(cmd);
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0173",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 74 传播场景->String操作->subSequence
//     */
//    @PostMapping(value = "case0173")
//    public Map<String, Object> aTaintCase0173(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            Runtime.getRuntime().exec(String.valueOf(cmd.subSequence(0,2)));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0174",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` /**
//     * 75 传播场景->String操作->substring
//     * lsabc
//     */
//    @PostMapping(value = "case0174")
//    public Map<String, Object> aTaintCase0174(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            Runtime.getRuntime().exec(cmd.substring(0,2));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		//传播场景->StringBuilder操作
//		{"aTaintCase0181",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` /**
//     *传播场景->StringBuilder操作->构造方法
//     */
//    @PostMapping(value = "case0181")
//    public Map<String, Object> aTaintCase0181(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            Runtime.getRuntime().exec(String.valueOf(new StringBuilder(cmd)));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }
//    `},
//		{"aTaintCase0182",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` /**
//     *传播场景->StringBuilder操作->append
//     */
//    @PostMapping(value = "case0182")
//    public Map<String, Object> aTaintCase0182(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            Runtime.getRuntime().exec(builder.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0183",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     *传播场景->StringBuilder操作->charAt
//     *
//     */
//    @PostMapping(value = "case0183")
//    public Map<String, Object> aTaintCase0183(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            char c= builder.charAt(0);
//            Runtime.getRuntime().exec(String.valueOf(c));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0184",true, []string{"Parameter-cmd", "Undefined-Runtime",}, `  /**
//     * 传播场景->StringBuilder操作->delete
//     */
//    @PostMapping(value = "case0184")
//    public Map<String, Object> aTaintCase0184(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            builder.delete(2,cmd.length());
//            Runtime.getRuntime().exec(builder.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0185",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 传播场景->StringBuilder操作->deleteCharAt
//     */
//    @PostMapping(value = "case0185")
//    public Map<String, Object> aTaintCase0185(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            builder.deleteCharAt(2);
//            Runtime.getRuntime().exec(builder.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0186",true, []string{"Parameter-cmd", "Undefined-Runtime",}, `  /**
//     * 传播场景->StringBuilder操作->getChars
//     */
//    @PostMapping(value = "case0186")
//    public Map<String, Object> aTaintCase0186(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            char[] chars = {0,0};
//            builder.getChars(0,2,chars,0);
//            Runtime.getRuntime().exec(String.valueOf(chars));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0187",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 传播场景->StringBuilder操作->insert
//     */
//    @PostMapping(value = "case0187")
//    public Map<String, Object> aTaintCase0187(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.insert(0,cmd);
//            Runtime.getRuntime().exec(builder.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0188",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 传播场景->StringBuilder操作->replace
//     */
//    @PostMapping(value = "case0188")
//    public Map<String, Object> aTaintCase0188(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder("abc");
//            builder.replace(2,3,cmd);
//            Runtime.getRuntime().exec(builder.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0189",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 传播场景->StringBuilder操作->subSequence
//     */
//    @PostMapping(value = "case0189")
//    public Map<String, Object> aTaintCase0189(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            builder.subSequence(0,2);
//            Runtime.getRuntime().exec(String.valueOf(builder.subSequence(0,2)));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0190",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 传播场景->StringBuilder操作->subString
//     */
//    @PostMapping(value = "case0190")
//    public Map<String, Object> aTaintCase0190(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            builder.substring(0,2);
//            Runtime.getRuntime().exec(builder.substring(0,2));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
// {"aTaintCase0191",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 传播场景->StringBuilder操作->toString
//     */
//    @PostMapping(value = "case0191")
//    public Map<String, Object> aTaintCase0191(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            StringBuilder builder = new StringBuilder();
//            builder.append(cmd);
//            Runtime.getRuntime().exec(builder.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0192",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` /**
//     * 传播场景-char[],byte[]操作->copyOf
//     */
//    @PostMapping(value = "case0192")
//    public Map<String, Object> aTaintCase0192(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            byte[] b1 = cmd.getBytes();
//            byte[] b2 = Arrays.copyOf(b1,10);
//            Runtime.getRuntime().exec(new String(b2));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`},
//		{"aTaintCase0193",true, []string{"Parameter-cmd", "Undefined-Runtime",}, ` /**
//     * 传播场景-char[],byte[]操作-->copyOfRange
//     */
//    @PostMapping(value = "case0193")
//    public Map<String, Object> aTaintCase0193(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            byte[] b1 = cmd.getBytes();
//            byte[] b2 = Arrays.copyOfRange(b1,0,2);
//            Runtime.getRuntime().exec(new String(b2));
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }`,
//   {"aTaintCase0194",true, []string{"Parameter-cmd", "Undefined-Runtime",},
//  `/**
//     * 传播场景-char[],byte[]操作->toString
//     */
//    @PostMapping(value = "case0194")
//    public Map<String, Object> aTaintCase0194(@RequestParam String cmd) {
//        Map<String, Object> modelMap = new HashMap<>();
//        try {
//            char[] chars = cmd.toCharArray();
//            Runtime.getRuntime().exec(chars.toString());
//            modelMap.put("status", "success");
//        } catch (Exception e) {
//            modelMap.put("status", "error");
//        }
//        return modelMap;
//    }
//`},},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			tt.code = createAllCode(tt.code)
//			target, err := getExecTopDef(tt.code)
//			if err != nil {
//				t.Fatal("prog parse fail", err)
//			}
//			if !strings.Contains(target, tt.target) {
//				t.Fatalf("want to get source [%v],but got [%v].", tt.target, target)
//			}
//		})
//	}
//}

func Test_CrossClass_Simple_Exec_Case(t *testing.T) {
	tests := []struct {
		name   string
		equal  bool
		expect []string
		code   string
	}{
		{"aTaintCase013", true,
			[]string{"\"|grep a\"", "Parameter-cmd", "Undefined-Runtime"},
			`  /**
    * MethodInvocation
    */
   @GetMapping("case013/{cmd}")
   public Map<String, Object> aTaintCase013(@PathVariable String cmd) {
       Map<String, Object> modelMap = new HashMap<>();
		CmdUtil cmdUtil = new CmdUtil();
       try {
           cmdUtil.run(cmd+"|grep a");
           modelMap.put("status", "success");
       } catch (Exception e) {
           modelMap.put("status", "error");
       }
       return modelMap;
   }`}, //方法调用
		{"aTaintCase014", false,
			[]string{
				"Parameter-cmd",
				`"www.test.com"`,
			},
			` /**
    * MethodInvocation+InfixExpression
    */
   @GetMapping("case014/{cmd}")
   public Map<String, Object> aTaintCase014(@PathVariable String cmd) {
       Map<String, Object> modelMap = new HashMap<>();
		CmdUtil cmdUtil = new CmdUtil();
       try {
           cmdUtil.run(cmd+ HttpUtil.doGet("www.test.com"));
           modelMap.put("status", "success");
       } catch (Exception e) {
           modelMap.put("status", "error");
       }
       return modelMap;
   }`}, //方法调用+中缀表达式
		{"aTaintCase015", false,
			[]string{
				"Parameter-cmd",
				// "Parameter-path",
				`"www.test.com"`,
			},
			`  
    /**
    * ifStatement
    */
   @GetMapping("case015/{cmd}")
   public Map<String, Object> aTaintCase015(@PathVariable String cmd) {
       Map<String, Object> modelMap = new HashMap<>();
		CmdUtil cmdUtil = new CmdUtil();
       try {
           if(true == false){
               cmdUtil.run(cmd);
           }else{
               String cmdString = HttpUtil.doGet("www.test.com");
               cmdUtil.run(cmd+cmdString);
           }
           modelMap.put("status", "success");
       } catch (Exception e) {
           modelMap.put("status", "error");
       }
       return modelMap;
   }`}, //if语句
		{"aTaintCase016", true, []string{"\" \"", "\"mkdir\"", "Parameter-cmd", "Undefined-Runtime"}, ` /**
    * Switch
    */
   @GetMapping("case016/{type}/{cmd}")
   public Map<String, Object> aTaintCase016(@PathVariable String cmd,@PathVariable String type) {
       Map<String, Object> modelMap = new HashMap<>();
		CmdUtil cmdUtil = new CmdUtil();
       try {
           switch (type){

               case "mkdir":
                   cmdUtil.run("mkdir"+" "+cmd);
                   modelMap.put("status", "success");
               default:
                   modelMap.put("status", "success");
                   return null;
           }

       } catch (Exception e) {
           modelMap.put("status", "error");
       }
       return modelMap;
   }`}, //switch语句

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := createCmdUtilCode(tt.code)
			testExecTopDef(t, &TestCase{
				Code:    code,
				Contain: !tt.equal,
				Expect: map[string][]string{
					"target": tt.expect,
				},
			})
		})
	}

}

func createCmdUtilCode(code string) string {
	CmdUtilCode := fmt.Sprintf(`package com.sast.astbenchmark.common.utils;
public class CmdUtil {
    public void run(String path) {
        try {
            Runtime.getRuntime().exec(path);
        }catch (Exception e) {
            return;
        }
    }
}
@RestController()
public class AstTaintCase001 {
%v
}`, code)
	return CmdUtilCode
}

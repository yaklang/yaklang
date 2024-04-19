package java

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"strings"
	"testing"
)

func Test_Simple_Exec_Case(t *testing.T) {
	tests := []struct {
		name   string
		target string
		code   string
	}{
		{"aTaintCase011", "Parameter-cmd", `@GetMapping("case011/{cmd}")
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
		{"aTaintCase012", "Parameter-cmd", `@GetMapping("case011/{cmd}")
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
		{"aTaintCase0110", "Parameter-request", ` @PostMapping("case0110")
    public Map<String, Object> aTaintCase0110(@RequestParam("cmd") String cmd, HttpServletRequest request) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String cmdStr = request.getParameterMap().get("cmd")[0];
            Runtime.getRuntime().exec(cmdStr);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0111", "Parameter-request", ` /**
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
    }`},
		{"aTaintCase0112", "ls -la", `/**
     * argument arrayaccess
     * @param
     * @return
     */
    @PostMapping(value = "case0112")
    public Map<String, Object> aTaintCase0112(@RequestParam(defaultValue = "ls") String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            String strs[] = new String[1];
            strs[0]=cmd;
            List<String> target = Lists.newArrayList("cd /","ls","ls -la");
            CollectionUtils.mergeArrayIntoCollection(strs,target);
            Runtime.getRuntime().exec(target.get(3));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }

    }`},
		{"aTaintCase0113", "make(any)", `/**
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
		{"aTaintCase0117", "Parameter-cmd", ` /**
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
		{"aTaintCase0118", "Parameter-cmd", `    /**
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
		{"aTaintCase0127", "Parameter-cmd", ` /**
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
		{"aTaintCase0128", "Parameter-cmd", `/**
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
		{"aTaintCase0129", "Parameter-cmd", `   /**
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
		{"aTaintCase0134", "Parameter-cmd", `/**
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
		{"aTaintCase0135", "Parameter-cmd", `
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
		{"aTaintCase0136", "Parameter-cmd", ` /**
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
		{"aTaintCase0137", "Parameter-cmd", `/**
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
		{"aTaintCase0138", "Parameter-cmd", `  /**
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
		{"aTaintCase0139", "Parameter-cmd", `/**
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
		{"aTaintCase0140", "Parameter-cmd", `  /**
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
		{"aTaintCase0141", "Parameter-cmd", `
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
		{"aTaintCase0144", "Parameter-cmd", ` /**
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
		{"aTaintCase0145", "Parameter-cmd", `/**
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
		{"aTaintCase0146", "Parameter-cmd", `
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
		{"aTaintCase0147", "Parameter-cmd", ` /**
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
		{"aTaintCase0149", "Parameter", `/**
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
		{"aTaintCase0150", "Parameter", `/**
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
		{"aTaintCase0151", "Parameter-cmd", ` /**
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
		{"aTaintCase0152", "Parameter-cmd", `    /**
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
		{"aTaintCase0153", "Parameter-cmd", `    /**
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

		{"aTaintCase0155", "Parameter-cmd", ` /**
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
		{"aTaintCase0158", "ls", `/**
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
		{"aTaintCase0159", "Parameter-cmd", `/**
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
		{"aTaintCase0161", "Parameter-cmd", `  /**
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
		{"aTaintCase0162", "Parameter-cmd", `  /**
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
		{"aTaintCase0163", "Parameter-cmd", ` /**
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
		{"aTaintCase0164", "Parameter-cmd", `/**
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

		{"aTaintCase0166", "Parameter-cmd", `/**
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
		{"aTaintCase0167", "Parameter-cmd", `/**
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
		{"aTaintCase0168", "Parameter-cmd", `
    /**
     * 70 传播场景->String操作->repeat
     *
     */
    @PostMapping(value = "case0168")
    public Map<String, Object> aTaintCase0168(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {

            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0171", "Parameter-cmd", ` /**
     * 72 传播场景->String操作->split
     */
    @PostMapping(value = "case0171")
    public Map<String, Object> aTaintCase0171(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.split(" ")[0];
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0172", "Parameter-cmd", `/**
     * 73 传播场景->String操作->strip
     */
    @PostMapping(value = "case0172")
    public Map<String, Object> aTaintCase0172(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0175", "Parameter-cmd", ` /**
     * 76 传播场景->String操作->toCharArray
     */
    @PostMapping(value = "case0175")
    public Map<String, Object> aTaintCase0175(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            char[] chars=cmd.toCharArray();
            Runtime.getRuntime().exec(String.valueOf(chars));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0176", "Parameter-cmd", `/**
     * 77 传播场景->String操作->toLowerCase
     */
    @PostMapping(value = "case0176")
    public Map<String, Object> aTaintCase0176(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.toLowerCase();
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0177", "Parameter-cmd", ` /**
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
		{"aTaintCase0178", "Parameter-cmd", `/**
     *传播场景->String操作->toUpperCase
     */
    @PostMapping(value = "case0178")
    public Map<String, Object> aTaintCase0178(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.toUpperCase();
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0179", "Parameter-cmd", `/**
     *传播场景->String操作->trim
     */

    @PostMapping(value = "case0179")
    public Map<String, Object> aTaintCase0179(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.trim();
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0180", "Parameter-cmd", ` /**
     * 传播场景->String操作->valueOf
     */
    @PostMapping(value = "case0180")
    public Map<String, Object> aTaintCase0180(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=String.valueOf(cmd);
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0194", "Parameter-cmd", `/**
     * 传播场景-char[],byte[]操作->toString
     */
    @PostMapping(value = "case0194")
    public Map<String, Object> aTaintCase0194(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            char[] chars = cmd.toCharArray();
            Runtime.getRuntime().exec(chars.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
		{"aTaintCase0195", "Parameter", ` /**
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
    }`}, // 参数为数组的时候，参数名为Parameter-#形式，而非cmd
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.code = createAllCode(tt.code)
			target, err := getExecTopDef(tt.code)
			if err != nil {
				t.Fatal("prog parse fail", err)
			}
			if !strings.Contains(target, tt.target) {
				t.Fatalf("want to get source [%v],but got [%v].", tt.target, target)
			}
		})
	}
}

func Test_Special_Exec_Case(t *testing.T) {
	tests := []struct {
		name   string
		target string
		code   string
	}{
		{"aTaintCase018", "Parameter-cmd", `@PostMapping("case018/{cmd}")
   public Map<String, Object> aTaintCase018(@PathVariable String cmd) {
       Map<String, Object> modelMap = new HashMap<>();
       if (cmd == null) {
           modelMap.put("status", "error");
           return modelMap;
       }
       try {
           String[] b = {"a","b"};
           System.arraycopy(cmd,0,b,0,2);
           Runtime.getRuntime().exec(b[0]);
           modelMap.put("status", "success");
       } catch (Exception e) {
           modelMap.put("status", "error");
       }
       return modelMap;
   }
`}, //System.arraycopy方法
		{"aTaintCase019", "Parameter-cmd", ` @PostMapping("case019")
    public Map<String, Object> aTaintCase019(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        char[] data = cmd.toCharArray();
        try {
            Runtime.getRuntime().exec(new String(data));
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`}, // 实例化一个不存在的类
		{"aTaintCase0114", "Parameter-cmd", `  /**
     * MI+MI
     * @param cmd
     * @return
     */
    @PostMapping(value = "case0114")
    public Map<String, Object> aTaintCase0114(@RequestParam(defaultValue = "ls") String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd.toUpperCase());
            Runtime.getRuntime().exec(builder.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`}, //StringBuilder
		{"aTaintCase0115", "Parameter-cmd", `/**
     * MI+arguement
     */
    @PostMapping(value = "case0115")
    public Map<String, Object> aTaintCase0115(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            char[] chars= new char[]{0,0};
            cmd.getChars(0,2,chars,0);
            Runtime.getRuntime().exec(String.valueOf(chars));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`}, //getChars
		{"aTaintCase0142", "Parameter-cmd", `/**
     * 引用类型queue 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0142")
    public Map<String, Object> aTaintCase0142(@RequestBody List<String> cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null || CollectionUtils.isEmpty(cmd)) {
            modelMap.put("status", "error");
            return modelMap;
        }
        Queue<String> queue = new LinkedBlockingQueue();
        try {
            queue.add(cmd.get(0));
            Runtime.getRuntime().exec(queue.peek());
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`}, //引用类型queue 作为污点源
		{"aTaintCase0143", "Parameter-cmd", ` /**
     * 引用类型Set 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0143")
    public Map<String, Object> aTaintCase0143(@RequestBody List<String> cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null || CollectionUtils.isEmpty(cmd)) {
            modelMap.put("status", "error");
            return modelMap;
        }
        Set<String> stringSet = new HashSet<>(cmd);
        try {

            Runtime.getRuntime().exec(stringSet.stream().iterator().next());
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`}, //引用类型Set 作为污点源
		{"aTaintCase0154", "Parameter-cmd", `/**
     * 其他对象 StringBuilder 作为污点源
     *
     * @param cmd
     * @return
     */
    @PostMapping("case0154")
    public Map<String, Object> aTaintCase0154(@RequestBody String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        if (cmd == null) {
            modelMap.put("status", "error");
            return modelMap;
        }
        StringBuilder data = new StringBuilder();
        data.append(cmd);
        try {
            Runtime.getRuntime().exec(data.toString());
            modelMap.put("status", "success");
        } catch (IOException e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }

`}, // StringBuilder 作为污点源
		{"aTaintCase0160", "Parameter-cmd", ` /**
     * 62 传播场景->String操作->构造方法
     */
    @PostMapping(value = "case0160")
    public Map<String, Object> aTaintCase0160(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(new String(cmd));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0165", "Parameter-cmd", `/**
     * 67 传播场景->String操作->getChars
     */
    @PostMapping(value = "case0165")
    public Map<String, Object> aTaintCase0165(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            char[] chars= new char[]{0,0};
            cmd.getChars(0,2,chars,0);
            Runtime.getRuntime().exec(String.valueOf(chars));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0169", "Parameter-cmd", `  /**
     * 71 传播场景->String操作->replace
     * ls;-la
     */
    @PostMapping(value = "case0169")
    public Map<String, Object> aTaintCase0169(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.replace(";"," ");
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
`},
		{"aTaintCase0170", "Parameter-cmd", `    /**
     *  传播场景->String操作->replace
     * alasa
     */
    @PostMapping(value = "case0170")
    public Map<String, Object> aTaintCase0170(@RequestParam String cmd ) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            cmd=cmd.replaceAll("a","");
            Runtime.getRuntime().exec(cmd);
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0173", "Parameter-cmd", `/**
     * 74 传播场景->String操作->subSequence
     */
    @PostMapping(value = "case0173")
    public Map<String, Object> aTaintCase0173(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(String.valueOf(cmd.subSequence(0,2)));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0174", "Parameter-cmd", ` /**
     * 75 传播场景->String操作->substring
     * lsabc
     */
    @PostMapping(value = "case0174")
    public Map<String, Object> aTaintCase0174(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(cmd.substring(0,2));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		//传播场景->StringBuilder操作
		{"aTaintCase0181", "Parameter-cmd", ` /**
     *传播场景->StringBuilder操作->构造方法
     */
    @PostMapping(value = "case0181")
    public Map<String, Object> aTaintCase0181(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            Runtime.getRuntime().exec(String.valueOf(new StringBuilder(cmd)));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }
    `},
		{"aTaintCase0182", "Parameter-cmd", ` /**
     *传播场景->StringBuilder操作->append
     */
    @PostMapping(value = "case0182")
    public Map<String, Object> aTaintCase0182(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            Runtime.getRuntime().exec(builder.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0183", "Parameter-cmd", `/**
     *传播场景->StringBuilder操作->charAt
     *
     */
    @PostMapping(value = "case0183")
    public Map<String, Object> aTaintCase0183(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            char c= builder.charAt(0);
            Runtime.getRuntime().exec(String.valueOf(c));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0184", "Parameter-cmd", `  /**
     * 传播场景->StringBuilder操作->delete
     */
    @PostMapping(value = "case0184")
    public Map<String, Object> aTaintCase0184(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            builder.delete(2,cmd.length());
            Runtime.getRuntime().exec(builder.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0185", "Parameter-cmd", `/**
     * 传播场景->StringBuilder操作->deleteCharAt
     */
    @PostMapping(value = "case0185")
    public Map<String, Object> aTaintCase0185(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            builder.deleteCharAt(2);
            Runtime.getRuntime().exec(builder.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0186", "Parameter-cmd", `  /**
     * 传播场景->StringBuilder操作->getChars
     */
    @PostMapping(value = "case0186")
    public Map<String, Object> aTaintCase0186(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            char[] chars = {0,0};
            builder.getChars(0,2,chars,0);
            Runtime.getRuntime().exec(String.valueOf(chars));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0187", "Parameter-cmd", `/**
     * 传播场景->StringBuilder操作->insert
     */
    @PostMapping(value = "case0187")
    public Map<String, Object> aTaintCase0187(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.insert(0,cmd);
            Runtime.getRuntime().exec(builder.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0188", "Parameter-cmd", `/**
     * 传播场景->StringBuilder操作->replace
     */
    @PostMapping(value = "case0188")
    public Map<String, Object> aTaintCase0188(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder("abc");
            builder.replace(2,3,cmd);
            Runtime.getRuntime().exec(builder.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0189", "Parameter-cmd", `/**
     * 传播场景->StringBuilder操作->subSequence
     */
    @PostMapping(value = "case0189")
    public Map<String, Object> aTaintCase0189(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            builder.subSequence(0,2);
            Runtime.getRuntime().exec(String.valueOf(builder.subSequence(0,2)));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0190", "Parameter-cmd", `/**
     * 传播场景->StringBuilder操作->subString
     */
    @PostMapping(value = "case0190")
    public Map<String, Object> aTaintCase0190(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            builder.substring(0,2);
            Runtime.getRuntime().exec(builder.substring(0,2));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0191", "Parameter-cmd", `/**
     * 传播场景->StringBuilder操作->toString
     */
    @PostMapping(value = "case0191")
    public Map<String, Object> aTaintCase0191(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            StringBuilder builder = new StringBuilder();
            builder.append(cmd);
            Runtime.getRuntime().exec(builder.toString());
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0192", "Parameter-cmd", ` /**
     * 传播场景-char[],byte[]操作->copyOf
     */
    @PostMapping(value = "case0192")
    public Map<String, Object> aTaintCase0192(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            byte[] b1 = cmd.getBytes();
            byte[] b2 = Arrays.copyOf(b1,10);
            Runtime.getRuntime().exec(new String(b2));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
		{"aTaintCase0193", "Parameter-cmd", ` /**
     * 传播场景-char[],byte[]操作-->copyOfRange
     */
    @PostMapping(value = "case0193")
    public Map<String, Object> aTaintCase0193(@RequestParam String cmd) {
        Map<String, Object> modelMap = new HashMap<>();
        try {
            byte[] b1 = cmd.getBytes();
            byte[] b2 = Arrays.copyOfRange(b1,0,2);
            Runtime.getRuntime().exec(new String(b2));
            modelMap.put("status", "success");
        } catch (Exception e) {
            modelMap.put("status", "error");
        }
        return modelMap;
    }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.code = createAllCode(tt.code)
			target, err := getExecTopDef(tt.code)
			if err != nil {
				t.Fatal("prog parse fail", err)
			}
			if !strings.Contains(target, tt.target) {
				t.Fatalf("want to get source [%v],but got [%v].", tt.target, target)
			}
		})
	}
}

func Test_CrossClass_Simple_Exec_Case(t *testing.T) {
	tests := []struct {
		name   string
		target string
		isSink bool
		code   string
	}{
		{"aTaintCase013", "Parameter-cmd", true, ` /**
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
		{"aTaintCase014", "Parameter-cmd", true, ` /**
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
		{"aTaintCase015", "Parameter-cmd", true, ` /**
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
		{"aTaintCase016", "Parameter-cmd", true, ` /**
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
			tt.code = createCmdUtilCode(tt.code)
			target, err := getExecTopDef(tt.code)
			if err != nil {
				t.Fatal("prog parse fail", err)
			}
			if !strings.Contains(target, tt.target) {
				t.Fatalf("want to get source [%v],but got [%v].", tt.target, target)
			}

		})
	}

}

func Test_CrossClass_SideEffect_Exec_Case(t *testing.T) {
	tests := []struct {
		name   string
		target string
		isSink bool
		code   string
	}{
		{"aTaintCase022", "Function-CmdObject_setCmd(make(CmdObject),Parameter-cmd)", true, `/**
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
		{"aTaintCase022_2", "cd /", false, ` /**
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
`}, //todo topdef得到结果过多
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.code = createCmdObject(tt.code)
			target, err := getExecTopDef(tt.code)
			if err != nil {
				t.Fatal("prog parse fail", err)
			}
			if !strings.Contains(target, tt.target) {
				t.Fatalf("want to get source [%v],but got [%v].", tt.target, target)
			}
		})
	}
}

func getExecTopDef(code string) (string, error) {
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage("java"))
	if err != nil {
		return "", err
	}
	prog.Show()
	runtime := prog.Ref("Runtime").Ref("getRuntime")[0].GetCalledBy()
	exec := runtime.Ref("exec")[0].GetCalledBy()
	args := exec[0].GetCallArgs()
	topDef := args.GetTopDefs(ssaapi.WithDepthLimit(100), ssaapi.WithAllowCallStack(true))
	topDef.ShowWithSource()
	target := topDef.StringEx(0)
	return target, nil
}

func createAllCode(code string) string {
	allCode := fmt.Sprintf(`package com.sast.astbenchmark.cases;
		@RestController()
	public class AstTaintCase{
	private SSRFShowManager ssrfShowManager = new SSRFShowManageImpl();
	%v
	}`, code)
	return allCode
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

func createCmdObject(code string) string {
	CmdUtilCode := fmt.Sprintf(`package com.sast.astbenchmark.model;

public class CmdObject {
    private String cmd1;
    private String cmd2;
    private String cmd3;

    public void setCmd(String cmd) {
        this.cmd1 = cmd;
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

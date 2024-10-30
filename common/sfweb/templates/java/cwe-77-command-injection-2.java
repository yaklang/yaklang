package com.example;

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
@PostMapping(value = "Cross_Class_Command_Injection-1")
      public Map<String, Object> CrossClassTest1(@RequestParam String cmd) {
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
      }

   @PostMapping(value = "Cross_Class_Command_Injection-2")
         public Map<String, Object> CrossClassTest2(@RequestParam String cmd) {
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
}
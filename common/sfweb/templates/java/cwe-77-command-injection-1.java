package com.example.utils;
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
public class Cmd_Inj_Test {
    @GetMapping("reflect_as_sink")
    public Map<String, Object> ReflectAsSink(@PathVariable String cmd,@PathVariable String methodname) {
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
    }

     @PostMapping("map_as_sink")
        public Map<String, Object> MapAsSink(@RequestBody Map<String, String> cmd) {
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
        }

}
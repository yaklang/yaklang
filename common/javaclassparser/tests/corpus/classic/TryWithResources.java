import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;

public class TryWithResources {
    public int readOne(byte[] data) throws IOException {
        try (InputStream in = new ByteArrayInputStream(data)) {
            return in.read();
        }
    }

    public int readTwo(byte[] data) throws IOException {
        try (InputStream a = new ByteArrayInputStream(data);
             InputStream b = new ByteArrayInputStream(data)) {
            return a.read() + b.read();
        } catch (IOException e) {
            return -1;
        }
    }
}

import java.io.*;
import org.springframework.web.bind.annotation.*;

public class VulnerableClass {
    public static void main(String[] args) {
        try {
            ObjectInputStream ois = new ObjectInputStream(new FileInputStream("data.bin"));
            Object obj = ois.readObject();
            ois.close();
        } catch (IOException | ClassNotFoundException e) {
            e.printStackTrace();
        }
    }
}

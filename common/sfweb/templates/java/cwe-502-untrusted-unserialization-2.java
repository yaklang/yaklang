import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;
import java.io.*;

@RestController
public class VulnerableController {
    @PostMapping("/deserialize")
    public String deserializeObject(@RequestBody byte[] data) {
        try {
            ByteArrayInputStream bis = new ByteArrayInputStream(data);
            ObjectInputStream ois = new ObjectInputStream(bis);
            Object obj = ois.readObject();
            ois.close();
            return "Deserialization successful: " + obj.toString();
        } catch (IOException | ClassNotFoundException e) {
            e.printStackTrace();
            return "Error during deserialization: " + e.getMessage();
        }
    }
}

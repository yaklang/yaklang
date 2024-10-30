// demo 1
import java.io.*;

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

// demo 2
import org.springframework.web.bind.annotation.PostMapping;
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

// demo 3
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import java.io.*;

@WebServlet("/vulnerable")
public class VulnerableServlet extends HttpServlet {

    protected void doPost(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        try {
            ObjectInputStream ois = new ObjectInputStream(request.getInputStream());
            Object obj = ois.readObject();
            ois.close();

            response.getWriter().println("Deserialization successful: " + obj.toString());
        } catch (ClassNotFoundException e) {
            e.printStackTrace();
            response.getWriter().println("Error during deserialization: " + e.getMessage());
        }
    }
}
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
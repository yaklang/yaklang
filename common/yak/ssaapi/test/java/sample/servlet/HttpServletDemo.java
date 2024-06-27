import javax.servlet.*;
import javax.servlet.http.*;
import java.io.IOException;
import java.io.PrintWriter;

public class SimpleServlet extends HttpServlet {

    @Override
    protected void doGet(HttpServletRequest req, HttpServletResponse resp) throws ServletException, IOException {
        // 设置响应内容类型
        resp.setContentType("text/html");
        // 获取响应的 writer 对象，用于发送响应数据
        PrintWriter out = resp.getWriter();
        out.println("<h1>Hello, World from GET request!</h1>");
    }

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse resp) throws ServletException, IOException {
        // 设置响应内容类型
        resp.setContentType("text/html");
        // 从请求中获取参数
        String message = req.getParameter("message");
        // 获取响应的 writer 对象，用于发送响应数据
        PrintWriter out = resp.getWriter();
        out.println("<h1>Received POST request with message: " + message + "</h1>");
    }
}
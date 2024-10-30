// demo1
package com.example;

import jakarta.servlet.*;
import jakarta.servlet.http.*;
import java.io.*;

public class CommandInjectionServlet extends HttpServlet {
    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        String otherInput = request.getParameter("ccc");
        String userInput = request.getParameter("command");
        String command = "cmd.exe /c " + userInput; // 直接使用用户输入

        Process process = Runtime.getRuntime().exec(command);
        BufferedReader reader = new BufferedReader(new InputStreamReader(process.getInputStream()));
        String line;
        PrintWriter out = response.getWriter();

        while ((line = reader.readLine()) != null) {
            out.println(line);
        }
    }
}

// demo2

package com.example;

import jakarta.servlet.*;
import jakarta.servlet.http.*;
import java.io.*;

public class CommandInjectionServlet2 extends HttpServlet {
    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        String otherInput = request.getParameter("ccc");
        String userInput = request.getParameter("cmd");
        String command = "cmd.exe /c " + userInput; // 直接使用用户输入

        Process process = Runtime.getRuntime().exec(command);
        BufferedReader reader = new BufferedReader(new InputStreamReader(process.getInputStream()));
        String line;
        PrintWriter out = response.getWriter();

        while ((line = reader.readLine()) != null) {
            out.println(line);
        }
    }
}

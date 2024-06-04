package org.vuln.javasec.controller.basevul.file;

import org.vuln.javasec.util.Security;
import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.RequestMapping;

import javax.servlet.http.HttpServletResponse;
import java.io.*;
import java.nio.file.Files;

@Controller
@RequestMapping("/home/file")
public class DownloadVul {

    @RequestMapping("/download")
    public String readFile(String filename, String check, HttpServletResponse response, Model model) {
        if (check != null && check.equals("true")) {
            if (Security.checkTraversal(filename)) {
                model.addAttribute("results", "请勿输入非法文件名!");
                return "basevul/file/download";
            }
        }

        String filePath = System.getProperty("user.dir") + "/src/main/resources/static/upload/" + filename;

        try {
            File file = new File(filePath);
            InputStream fis = new BufferedInputStream(Files.newInputStream(file.toPath()));
            byte[] buffer = new byte[fis.available()];
            fis.read(buffer);
            fis.close();

            response.reset();
            response.addHeader("Content-Disposition", "attachment;filename=" + filename);
            response.addHeader("Content-Length", "" + file.length());
            OutputStream toClient = new BufferedOutputStream(response.getOutputStream());
            response.setContentType("application/octet-stream");
            toClient.write(buffer);
            toClient.flush();
            toClient.close();
            return "";
        } catch (Exception e) {
            e.printStackTrace();
            model.addAttribute("results", e.toString());
            return "basevul/file/download";
        }
    }
}
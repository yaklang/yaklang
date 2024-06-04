package org.vuln.javasec.controller.basevul.file;

import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.multipart.MultipartFile;

import javax.servlet.http.HttpServletRequest;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

@Controller
@RequestMapping("/home/file")
public class UploadVul {

    private static final String UPLOADED_FOLDER = System.getProperty("user.dir") + "/src/main/resources/static/upload/";

    @RequestMapping("/upload")
    public String uFile(@RequestParam("file") MultipartFile file, Model model, String check, HttpServletRequest httpServletRequest) {
        if (file.isEmpty()) {
            model.addAttribute("results", "请选择要上传的文件!");
            return "basevul/file/upload";
        }

        String fileName = file.getOriginalFilename();

        if (check.equals("true")) {

            // 获取文件后缀名，并且索引到最后一个，避免使用.jpg.jsp来绕过
            assert fileName != null;
            String Suffix = fileName.substring(fileName.lastIndexOf("."));

            String[] SuffixSafe = {".jpg", ".png", ".jpeg", ".gif", ".bmp", ".ico"};
            boolean flag = false;

            // 如果满足后缀名单，返回true
            for (String s : SuffixSafe) {
                if (Suffix.toLowerCase().equals(s)) {
                    flag = true;
                    break;
                }
            }
            if (!flag) {
                model.addAttribute("results", "请勿HACK! ");
                return "basevul/file/upload";
            }
        }

        try {
            byte[] bytes = file.getBytes();
            Path dir = Paths.get(UPLOADED_FOLDER);
            Path path = Paths.get(UPLOADED_FOLDER + fileName);

            if (!Files.exists(dir)) {
                Files.createDirectories(dir);
            }
            Files.write(path, bytes);

            String urlPath = httpServletRequest.getContextPath();
            String scheme = httpServletRequest.getScheme();
            String serverName = httpServletRequest.getServerName();
            int port = httpServletRequest.getServerPort();
            String basePath = scheme + "://" + serverName + ":" + port + urlPath;

            model.addAttribute("results", "上传成功" + System.lineSeparator() + "文件路径: " + path + System.lineSeparator() + "访问地址: " + basePath + "/upload/" + fileName);
        } catch (Exception e) {
            model.addAttribute("results", "上传失败: " + e);
        }
        return "basevul/file/upload";
    }
}
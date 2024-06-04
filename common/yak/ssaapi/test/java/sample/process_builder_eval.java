package org.vuln.javasec.controller.basevul.rce;

import org.vuln.javasec.util.Security;
import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.RequestMapping;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;

@Controller
@RequestMapping("/home/rce")
public class ProcessBuilderExec {
    @RequestMapping("/processbuilder")
    public String ProcessBuilderExec(String ip, String safe, Model model) {
        if (safe != null) {
            if (Security.checkOs(ip)) {
                model.addAttribute("results", "检测到非法命令注入");
                return "basevul/rce/processbuilder";
            }
        }
//        String[] cmdList = {"sh", "-c", "ping -c 1 " + ip};
        String[] cmdList = {"cmd", "/c", "ping -n 1 " + ip};
        StringBuilder sb = new StringBuilder();
        String line;
        String results;

        // 利用指定的操作系统程序和参数构造一个进程生成器
        ProcessBuilder pb = new ProcessBuilder(cmdList);
        pb.redirectErrorStream(true);

        // 使用此进程生成器的属性启动一个新进程
        Process process = null;
        try {
            process = pb.start();
            // 取得命令结果的输出流
            InputStream fis = process.getInputStream();
            // 用一个读输出流类去读
            InputStreamReader isr = new InputStreamReader(fis, "GBK");
            // 用缓存器读行
            BufferedReader br = new BufferedReader(isr);
            //直到读完为止
            while ((line = br.readLine()) != null) {
                sb.append(line).append(System.lineSeparator());
            }
            results = sb.toString();
        } catch (IOException e) {
            e.printStackTrace();
            results = e.toString();
        }
        model.addAttribute("results", results);
        return "basevul/rce/processbuilder";
    }
}
package org.vuln.javasec.controller.basevul.rce;

import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.RequestMapping;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;

@Controller
@RequestMapping("/home/rce")
public class RuntimeExec {
    @RequestMapping("/runtime")
    public String RuntimeExec(String cmd, Model model) {
        StringBuilder sb = new StringBuilder();
        String line;

        try {
            Process proc = Runtime.getRuntime().exec(cmd);

            InputStream fis = proc.getInputStream();
            InputStreamReader isr = new InputStreamReader(fis, "GBK");
            BufferedReader br = new BufferedReader(isr);
            while ((line = br.readLine()) != null) {
                sb.append(line).append(System.lineSeparator());
            }

        } catch (IOException e) {
            e.printStackTrace();
            sb.append(e);
        }
        model.addAttribute("results", sb.toString());
        return "basevul/rce/runtime";
    }
}
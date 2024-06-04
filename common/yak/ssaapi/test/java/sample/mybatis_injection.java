package org.vuln.javasec.controller.basevul.sqli;

import org.vuln.javasec.model.Admin;
import org.vuln.javasec.service.InjectService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.RequestMapping;

import java.util.ArrayList;

@Controller
@RequestMapping("/home/sqli/mybatis")
public class MyBatis {
    @Autowired
    @SuppressWarnings("all")
    InjectService injectService;

    @RequestMapping("/orderby")
    public String orderBy(String field, Model model) {
        ArrayList<Admin> adminList;
        try {
            adminList = injectService.orderBy(field);
            model.addAttribute("userInfo", adminList);
        } catch (Exception e) {
            e.printStackTrace();
            model.addAttribute("results", e.toString());
        }
        return "basevul/sqli/mybatis_orderby";
    }

    @RequestMapping("/like")
    public String like(String username, Model model) {
        try {
            Admin admin = injectService.likeSearch(username);
            model.addAttribute("userInfo", admin);
        } catch (Exception e) {
            e.printStackTrace();
            model.addAttribute("results", e.toString());
        }
        return "basevul/sqli/mybatis_like";
    }

    @RequestMapping("/in")
    public String in(String ids, Model model) {
        try {
//            List<String> list = Arrays.asList(ids.split(","));
//            ArrayList<Admin> adminList = injectService.in(list);
            ArrayList<Admin> adminList = injectService.in(ids);
            model.addAttribute("userInfo", adminList);
        } catch (Exception e) {
            e.printStackTrace();
            model.addAttribute("results", e.toString());
        }
        return "basevul/sqli/mybatis_in";
    }

}
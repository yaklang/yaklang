// demo 1

import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import java.util.Arrays;
import java.util.List;

@Controller
public class VulnerableController {

    @GetMapping("/view")
    public String viewPage(@RequestParam String page, Model model) {
        // 这里直接使用用户提供的参数作为模板名，这是不安全的
        return page;
    }

    @GetMapping("/view2")
    public String viewPage2(@RequestParam String page, Model model) {
        // 这里试图通过简单的字符串检查来防御，但仍然不安全
        if (page.contains("blackword")) {
            return "error";
        }
        return page;
    }

    @GetMapping("/view3")
    public String viewPage3(@RequestParam String page, Model model) {
        // 这里尝试通过移除某些字符来"净化"输入，但仍然不安全
        String cleanedPage = page.replaceAll("[^a-zA-Z0-9]", "");
        return cleanedPage;
    }

    @GetMapping("/view4")
    public String viewPage4(@RequestParam String page, Model model) {
        // 这里尝试通过白名单来限制页面，但实现不当
        List<String> allowedPages = Arrays.asList("home", "about", "contact");
        if (allowedPages.contains(page.toLowerCase())) {
            return page; // 注意这里返回的是原始的 page，而不是小写版本
        }
        return "error";
    }

    @GetMapping("/view5")
    public String viewPage5(@RequestParam String page, Model model) {
        // 这里尝试通过长度限制来防御，但仍然不安全
        if (page.length() > 20) {
            return "error";
        }
        return page;
    }

    @GetMapping("/view6")
    public String viewPage6(@RequestParam String page, Model model) {
        // 这里尝试通过前缀检查来限制模板，但实现不当
        if (!page.startsWith("safe_")) {
            return "error";
        }
        return page.substring(5); // 移除 "safe_" 前缀
    }
}

// demo 2
package com.ibeetl.admin.console.web;

@Controller
public class OrgConsoleController {
    @GetMapping(MODEL + "/edit.do")
    @Function("org.edit")
    public ModelAndView edit(String id) {
    	ModelAndView view = new ModelAndView("/admin/org" + "/edit.html");
        CoreOrg org = orgConsoleService.queryById(id);
        view.addObject("org", org);
        return view;
    }
}

// demo 3

package com.ibeetl.admin.console.web;

@Controller
public class OrgConsoleController {
    @GetMapping(MODEL + "/edit.do")
    @Function("org.edit")
    public ModelAndView edit(String id) {
    	ModelAndView view = new ModelAndView("/admin/org/edit.html");
        CoreOrg org = orgConsoleService.queryById(id);
        view.addObject("org", org);
        return view;
    }

    @GetMapping(MODEL + "/edit.do")
    @Function("org.edit")
    public ModelAndView edit2(String id) {
    	ModelAndView view = new ModelAndView("/admin/org/edit2.html");
        view.addObject("org", id);
        return view;
    }
}
import org.springframework.web.bind.annotation.*;
import org.springframework.web.servlet.ModelAndView;
import org.springframework.web.util.HtmlUtils;

@RestController
@RequestMapping("/xss")
public class XSSController {

    @GetMapping
    public ModelAndView showForm() {
        return new ModelAndView("xssForm");
    }

    @PostMapping("/submit")
    public String handleSubmit(@RequestParam("userInput") String userInput) {
        return "处理后的输入: " + userInput;
    }

    @PostMapping("/submit1")
    public String handleSubmit1(@RequestParam("userInput") String safeInput) {
        // 对用户输入进行 HTML 转义以防止 XSS
        String sanitizedInput = HtmlUtils.htmlEscape(safeInput);
        return "处理后的输入: " + sanitizedInput;
    }

    @PostMapping("/submit2")
    public String handleSubmit2(@RequestParam("userInput") String abc) {
        // 对用户输入进行 HTML 转义以防止 XSS
        String input = callbysomeother(abc);
        return "处理后的输入: " + input;
    }
}
import org.springframework.web.bind.annotation.*;
import org.springframework.web.client.RestTemplate;

@RestController
public class SsrfVulnerableController {

    @GetMapping("/fetch-url")
    public String fetchUrl(@RequestParam("url") String url) {
        try {
            RestTemplate restTemplate = new RestTemplate();
            String result = restTemplate.getForObject(url, String.class);
            return result;
        } catch (Exception e) {
            return "Error: " + e.getMessage();
        }
    }
}

@RestController
public class SsrfVulnerableController {

    @GetMapping("/fetch-url")
    public String fetchUrl(@RequestParam("url") String url) {
        try {
            RestTemplate restTemplate = new RestTemplate();
            String result = restTemplate.getForObject(url + "?queryid=1", String.class);
            return result;
        } catch (Exception e) {
            return "Error: " + e.getMessage();
        }
    }
}
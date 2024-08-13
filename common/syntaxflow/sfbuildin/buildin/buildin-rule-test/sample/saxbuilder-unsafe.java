import org.jdom2.Document;
import org.jdom2.input.SAXBuilder;

import java.io.File;

public class XXEVulnerableExample {
    public static void main(String[] args) {
        try {
            SAXBuilder builder = new SAXBuilder();
            Document doc = builder.build(new File("vulnerable.xml")); // 假设这个 XML 文件包含 DTD
            // 处理文档...
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}
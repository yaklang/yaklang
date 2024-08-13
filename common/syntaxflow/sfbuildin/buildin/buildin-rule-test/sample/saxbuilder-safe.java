import org.jdom2.Document;
import org.jdom2.input.SAXBuilder;

import java.io.File;

public class XXEPreventionExample {
    public static void main(String[] args) {
        try {
            SAXBuilder builder = new SAXBuilder();
            builder.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true); // 禁止 DTD
            builder.setFeature("http://xml.org/sax/features/external-general-entities", false); // 禁止外部一般实体
            builder.setFeature("http://xml.org/sax/features/external-parameter-entities", false); // 禁止外部参数实体
            builder.setFeature("http://apache.org/xml/features/nonvalidating/load-external-dtd", false); // 禁止加载外部 DTD

            Document doc = builder.build(new File("safe.xml")); // 假设这个 XML 文件不包含 DTD
            // 处理文档...
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}
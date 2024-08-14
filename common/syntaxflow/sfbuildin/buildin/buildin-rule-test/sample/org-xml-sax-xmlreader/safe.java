import org.xml.sax.Attributes;
import org.xml.sax.SAXException;
import org.xml.sax.XMLReader;
import org.xml.sax.helpers.DefaultHandler;
import org.xml.sax.helpers.XMLReaderFactory;

import javax.xml.parsers.SAXParserFactory;

public class SAXParserExampleWithXXEProtection {
    public static void main(String[] args) {
        try {
            SAXParserFactory factory = SAXParserFactory.newInstance();
            // 启用安全特性以防止 XXE 攻击
            factory.setFeature("http://xml.org/sax/features/external-general-entities", false);
            factory.setFeature("http://xml.org/sax/features/external-parameter-entities", false);
            factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
            factory.setFeature("http://apache.org/xml/features/nonvalidating/load-external-dtd", false);

            // 创建 SAXParser 并获取 XMLReader
            XMLReader xmlReader = factory.newSAXParser().getXMLReader();
            MyHandler handler = new MyHandler();
            xmlReader.setContentHandler(handler);
            xmlReader.parse("example.xml"); // 替换为你的 XML 文件路径
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}

class MyHandler extends DefaultHandler {
    public void startElement(String uri, String localName, String qName, Attributes attributes) throws SAXException {
        System.out.println("Start element: " + qName);
    }

    public void endElement(String uri, String localName, String qName) throws SAXException {
        System.out.println("End element: " + qName);
    }

    public void characters(char[] ch, int start, int length) throws SAXException {
        System.out.println("Characters: " + new String(ch, start, length));
    }
}
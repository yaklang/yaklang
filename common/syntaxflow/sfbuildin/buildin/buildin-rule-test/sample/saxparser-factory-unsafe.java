import javax.xml.parsers.SAXParser;
import javax.xml.parsers.SAXParserFactory;
import org.xml.sax.Attributes;
import org.xml.sax.SAXException;
import org.xml.sax.helpers.DefaultHandler;

public class XXEDemo {
    public static void main(String[] args) {
        try {
            SAXParserFactory factory = SAXParserFactory.newInstance();
            SAXParser saxParser = factory.newSAXParser();

            DefaultHandler handler = new DefaultHandler() {
                public void startElement(String uri, String localName, String qName, Attributes attributes) throws SAXException {
                    System.out.println("Start Element: " + qName);
                }

                public void characters(char ch[], int start, int length) throws SAXException {
                    System.out.println("Content: " + new String(ch, start, length));
                }

                public void endElement(String uri, String localName, String qName) throws SAXException {
                    System.out.println("End Element: " + qName);
                }
            };

            // 解析包含 XXE 漏洞的 XML 文件
            saxParser.parse("xxe_example.xml", handler);
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}
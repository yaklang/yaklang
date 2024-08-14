import org.xml.sax.Attributes;
import org.xml.sax.SAXException;
import org.xml.sax.XMLReader;
import org.xml.sax.helpers.DefaultHandler;
import org.xml.sax.helpers.XMLReaderFactory;

public class SAXParserExample {
    public static void main(String[] args) {
        try {
            XMLReader xmlReader = XMLReaderFactory.createXMLReader();
            MyHandler handler = new MyHandler();
            xmlReader.setContentHandler(handler);
            xmlReader.parse("example.xml");
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
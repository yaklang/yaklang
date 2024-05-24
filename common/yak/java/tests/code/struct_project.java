package ${package};

public class A {


    protected char curChar;
    protected SimpleCharStream input_stream;

    public void A(Class aClass){
        Method[] methods = aClass.getDeclaredMethods();
        for (Method method : methods) {
            temp = processAnnotations(method);
            if (temp != null) {
                result.addAll(temp);
            }
        }
        
        while (curChar <= 32 && (0x100002600L & (1L << curChar)) != 0L)
            curChar = input_stream.BeginToken();
    }

    private class MyWriteBar {
        private int id;
        
        public int getId() {
            return id;
        }
        
        public void setBar(String name) {
            if ("Sams".equals(name))
                id = 1;
            else
                id = 999;
        }
    }
}

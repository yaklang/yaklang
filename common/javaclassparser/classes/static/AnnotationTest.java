package org.benf.cfr.reader;

@MyAnnotation(value="test")
public class AnnotationTest<@MyAnnotation(value="test1") T> {
    @MyAnnotation(value="field")
    String field;
    @MyAnnotation(value="test1")
    int main(@MyAnnotation(value="test1")int p){
        @MyAnnotation(value="test1")
        int a;
        String str = (@MyAnnotation(value="test1") String) "a";

        System.out.println("123");

        @MyAnnotation(value="test1")
        int v = 1;
        return 1;
    }
    @MyAnnotation(value="constructor")
    AnnotationTest(){}
}

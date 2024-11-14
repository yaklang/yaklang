package org.benf.cfr.reader;

import java.util.ArrayList;
import java.util.List;

public class LambdaTest {
    void main(){
        List a = new ArrayList<>();
        a.forEach((Object o)->{
            System.out.println(o);
        });
    }

}

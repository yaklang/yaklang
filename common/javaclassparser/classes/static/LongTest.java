package org.benf.cfr.reader;

import java.util.HashMap;

public class LongTest {
	void main() {
		HashMap var1 = new HashMap();
		var1.merge(Long.valueOf(1L),Long.valueOf(10L),Long::sum);
		var1.merge(Long.valueOf(1L),Long.valueOf(5L),Long::sum);
		System.out.println(var1.get(Long.valueOf(1L)));
	}
}
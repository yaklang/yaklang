package org.benf.cfr.reader;

import java.util.HashMap;

public class LongTest {
	public LongTest() {
	}
	void main() {
		HashMap var1 = new HashMap();
		var1.merge(Long.valueOf(1),Long.valueOf(10),Long::sum);
		var1.merge(Long.valueOf(1),Long.valueOf(5),Long::sum);
		System.out.println(var1.get(Long.valueOf(1)));
	}
}
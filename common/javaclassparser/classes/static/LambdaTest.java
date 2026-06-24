package org.benf.cfr.reader;

import java.util.ArrayList;

public class LambdaTest {
	// Fields
	 int a;

	public LambdaTest() {
		this.a = (this.a) + (1);
	}
	void main() {
		new ArrayList().forEach((Object l0) -> {
			int var1 = 1;
		});
		int var1 = 1;
		ArrayList var2 = new ArrayList();
		var2.add(Integer.valueOf(1));
		var2.forEach((Object l0) -> {
			System.out.println(l0);
		});
	}
}
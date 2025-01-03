package org.benf.cfr.reader;

import java.util.ArrayList;

public class LambdaTest {
	// Fields
	 int a;

	public LambdaTest() {
		this.a = (this.a) + (1);
	}
	void main() {
		(new ArrayList()).forEach((Object var4) -> {
			int var5 = 1;
		});
		int var1 = 1;
		ArrayList var2 = new ArrayList();
		(var2).add((Object)(Integer.valueOf(1)));
		(var2).forEach((Object var7) -> {
			System.out.println(var7);
		});
	}
}
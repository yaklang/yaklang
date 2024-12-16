package org.benf.cfr.reader;

import java.util.ArrayList;
public class LambdaTest {
	public LambdaTest() {
	}
	void main() {
		ArrayList var1 = new ArrayList();
		ArrayList var2 = var1;
		var2.forEach((Object var3) -> {
			int var4 = 1;
		});
		int var3 = 1;
	}
}
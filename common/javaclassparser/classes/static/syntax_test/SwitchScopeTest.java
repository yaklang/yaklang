package org.benf.cfr.reader;

public class SwitchScopeTest {
	public SwitchScopeTest() {
	}
	void main() {
		int var3;
		int var1 = 1;
		switch (var1){
		case 1:
			int var2 = 2;
			switch (var1){
			case 1:
				var3 = 3;
			case 2:
				int var4 = 4;
			default:

			}
		case 2:
			var3 = 5;
		default:
			return;
		}
	}
}
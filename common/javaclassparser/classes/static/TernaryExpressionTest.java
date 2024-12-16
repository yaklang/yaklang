package org.benf.cfr.reader;

public class TernaryExpressionTest {
	public TernaryExpressionTest() {
	}
	int getVar() {
		return 1;
	}
	void main() {
		int var1 = 1;
		int var2 = ((var1) == (2)) ? (this.getVar()) : (((var1) == (1)) ? (1) : (2));
		System.out.println(var2);
	}
}
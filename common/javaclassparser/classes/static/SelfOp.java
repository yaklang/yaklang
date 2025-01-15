package org.benf.cfr.reader;

public class SelfOp {
	// Fields
	 int value;

	public SelfOp() {
		this.value = 1;
	}
	void main() {
		int var1 = (this.value) + (1);
		this.value = var1;
		int var2 = var1;
		int var3 = (this.value) - (1);
		this.value = var3;
		int var4 = var3;
		int var5 = this.value;
		this.value = (var5) + (1);
		var2 = var5;
		int var6 = this.value;
		this.value = (var6) - (1);
		var4 = var6;
	}
}
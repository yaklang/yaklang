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
		var2 = this.value++;
		var4 = this.value--;
	}
}
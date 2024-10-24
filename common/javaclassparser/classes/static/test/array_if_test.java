package org.benf.cfr.reader;

public class Demo1 {
	public Demo1() {
	}
	public void main() {
		int[] var1 = new int[1];
		int[][] var2 = new int[1][2];
		int var3 = var2.length;
		int var4 = var1[1];
		Demo1[] var5 = new Demo1[1];
		var5[0] = new Demo1();
		Demo1 var6 = var5[0];
		Long.compare(1,2);
		int var7 = var3 > var4 ? 1 : 0;
		int var8 = 1;
		int var9 = var8 > 1 ? 1 : 0;
		if (var9){
			System.out.println(1)
		}else{
			System.out.println(2)
		}
		return;
	}
}
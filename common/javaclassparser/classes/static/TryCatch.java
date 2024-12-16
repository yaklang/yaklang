package org.benf.cfr.reader;

public class TryCatch {
	public TryCatch() {
	}
	void main() {
		int var1 = 1;
		try{
			var1 = 2;
			System.out.println(2);
		}catch(Exception var2){
			var1 = 3;
			System.out.println(1);
		}
		System.out.println(var1);
	}
}
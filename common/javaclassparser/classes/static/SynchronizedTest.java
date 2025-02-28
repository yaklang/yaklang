package org.benf.cfr.reader;

public class SynchronizedTest {
	void main() {
		String var1 = "1";
		String var2 = var1;
		synchronized(var1){
			var1 = "2";
			if ((var1) == ("a")){
				var1 = "0";
			}else{
				var1 = "3";
			}
		}
		var1 = "4";
	}
}
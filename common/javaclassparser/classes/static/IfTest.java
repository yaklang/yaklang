package org.benf.cfr.reader;

public class IfTest {
	void main() {
		int var1 = 1;
		if ((var1) > (1)){
			var1 = 2;
		}else{
			var1 = 3;
		}
		if ((var1) > (1)){
			var1 = 2;
		}
		if (((var1) <= (1)) && ((var1) <= (0))){

		}else{
			var1 = 2;
		}
		if (((var1) <= (1)) && ((var1) <= (0))){
			var1 = 3;
		}else{
			var1 = 2;
		}
	}
}
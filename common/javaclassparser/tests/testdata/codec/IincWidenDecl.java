package codec;

// Differential-execution regression battery for Bug W (iinc-target slot declared too narrow).
//
// An int local fed by `baload` carries the array's byte element type, so the decompiler infers the
// slot as `byte`. When that local is then updated with a non-±1 compound assignment that javac
// emits as `iinc` (`b += 256` -> `iinc_w slot,256`; commons-codec Base64.encode), the iinc desugars
// to `b = b + 256`. With a `byte` declaration that is a possible-lossy conversion that will not
// recompile. Because javac emits iinc ONLY for genuinely int locals (byte/char/short compound
// assignment uses iadd + i2b/i2c/i2s instead), the slot is provably int and must be declared int.
//
// Kill-switch JDEC_IINC_WIDEN_OFF=1 restores the pre-fix byte declaration so the load-bearing test
// can prove the widen is essential.
public class IincWidenDecl {

	// int slot fed by baload (byte inferred) then widened via `b += 256` (iinc) inside a loop.
	static long foldUnsigned(byte[] in, int n) {
		long acc = 1469598103934665603L;
		for (int i = 0; i < n; i++) {
			int b = in[i];
			if (b < 0) {
				b += 256;
			}
			acc = (acc * 1099511628211L) + b;
		}
		return acc;
	}

	// Same widen, but a subtracting non-±1 iinc (`b -= 200`), exercising the SUB desugar branch.
	static long foldShifted(byte[] in, int n) {
		long acc = 0;
		for (int i = 0; i < n; i++) {
			int b = in[i];
			if (b > 0) {
				b -= 200;
			}
			acc = (acc << 5) - acc + b;
		}
		return acc;
	}

	public static void main(String[] args) {
		byte[] a = {0, 1, -2, 3, -4, 127, -128, 64, -100, 99};
		long fp = 1469598103934665603L;
		fp = fp * 1099511628211L + foldUnsigned(a, a.length);
		fp = fp * 1099511628211L + foldShifted(a, a.length);
		System.out.println("fp=" + fp);
	}
}

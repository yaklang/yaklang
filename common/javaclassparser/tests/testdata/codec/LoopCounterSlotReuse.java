package codec;

// Differential-execution regression battery for Bug X (loop-counter iinc bound to the wrong slot
// incarnation).
//
// javac frequently reuses a now-dead loop counter's slot for a later local of a different type. In
// commons-codec Base64.decode the counter slot (int i) is reused AFTER the loop for a byte[] buffer.
// The decompiler keeps a single global slot->ref table mutated in DFS traversal order; when the
// after-loop byte[] store is visited before the in-loop iinc, GetVar(slot) returns the byte[]
// reincarnation, so the loop's `i++` rendered as `someByteArray++` -> "bad operand type byte[] for
// unary operator ++", an uncompilable method.
//
// The fix walks back from the iinc to its reaching int-category definition (mirroring the existing
// load-mismatch repair). Kill-switch JDEC_IINC_REACHING_OFF=1 restores the pre-fix mis-binding, so
// the load-bearing test can prove the repair is essential.
public class LoopCounterSlotReuse {

	// Mirror Base64.decode shape: loop counter i lives in a slot reused for a byte[] after the loop;
	// the i++ sits at a merge point that several nested forward branches jump to, and there is a
	// mid-loop break (like the pad short-circuit).
	static int scan(byte[] in, int n, int sentinel) {
		int sum = 0;
		int pos = 0;
		for (int i = 0; i < n; i++) {
			byte[] buffer = new byte[4];
			byte b = in[pos++];
			if (b == sentinel) {
				sum = -sum - 1;
				break;
			}
			if (b >= 0 && b < in.length) {
				byte r = in[b % in.length];
				if (r >= 0 && (sum & 1) == 0) {
					buffer[0] = r;
					sum += buffer[0];
				}
			}
		}
		if (sum != 0) {
			byte[] buffer = new byte[2];
			buffer[0] = 9;
			sum += buffer[0];
		}
		return sum;
	}

	public static void main(String[] args) {
		byte[] a = {3, 7, 0, 5, 2, 9, 1, 4};
		long fp = 1469598103934665603L;
		fp = fp * 1099511628211L + scan(a, a.length, 99);
		fp = fp * 1099511628211L + scan(a, a.length, 0);
		fp = fp * 1099511628211L + scan(a, 4, 7);
		fp = fp * 1099511628211L + scan(a, 0, 1);
		System.out.println("fp=" + fp);
	}
}

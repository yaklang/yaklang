package codec;

// Differential-execution regression battery for jump-entered try-catch reconstruction.
//
// Each method's protected (try) region is ENTERED VIA A JUMP rather than by linear fall-through:
//   - guardThenParse: a `return` guard precedes the try, so the try body is reached only on the
//     false edge of `s == null` (the commons-codec QCodec.encode / decode shape).
//   - sumParsable: the try is a loop body, re-entered by the loop back-edge every iteration.
//   - classify: the try is the body of an else branch.
//   - decodeUtf8: a guard precedes a try that catches a CHECKED exception.
//
// For all of these the try-start opcode begins a fresh CFG walk with pre==nil, so the inline
// try-catch anchor in ScanJmp was skipped: the handler lost its only predecessor edge and
// DropUnreachableOpcode pruned it, deleting the whole try together with its catch. That turned a
// guarded call into one that propagates the exception (behavioral divergence at runtime) or, for the
// checked-exception case, into source that does not even recompile ("unreported exception").
//
// If anchorJumpEnteredTryCatch regresses, this battery breaks loudly: classify(0)/guardThenParse("xyz")
// throw out of main (run failure) and decodeUtf8 fails to recompile.
public class JumpEnteredTryCatch {

	static int guardThenParse(String s) {
		if (s == null) {
			return -1;
		}
		try {
			return Integer.parseInt(s);
		} catch (NumberFormatException e) {
			return -2;
		}
	}

	static int sumParsable(String[] xs) {
		int acc = 0;
		for (int i = 0; i < xs.length; i++) {
			try {
				acc += Integer.parseInt(xs[i]);
			} catch (NumberFormatException e) {
				acc -= 1;
			}
		}
		return acc;
	}

	static int classify(int n) {
		if (n < 0) {
			return 0;
		} else {
			try {
				return 100 / n;
			} catch (ArithmeticException e) {
				return -999;
			}
		}
	}

	static String decodeUtf8(byte[] b, boolean skip) {
		if (skip) {
			return "skip";
		}
		try {
			return new String(b, "UTF-8");
		} catch (java.io.UnsupportedEncodingException e) {
			throw new RuntimeException(e);
		}
	}

	public static void main(String[] args) {
		long acc = 1125899906842597L;
		acc = acc * 131 + guardThenParse(null);
		acc = acc * 131 + guardThenParse("123");
		acc = acc * 131 + guardThenParse("xyz");
		acc = acc * 131 + sumParsable(new String[]{"1", "2", "bad", "4"});
		acc = acc * 131 + classify(-5);
		acc = acc * 131 + classify(0);
		acc = acc * 131 + classify(4);
		acc = acc * 131 + decodeUtf8(new byte[]{104, 105}, false).length();
		acc = acc * 131 + decodeUtf8(new byte[]{1, 2, 3}, true).length();
		System.out.println("fp=" + acc);
	}
}

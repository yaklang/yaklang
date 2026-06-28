package codec;

// Differential-execution regression battery for the int-computational-category recovery fixes:
//   1. local-slot merge: a single slot conditionally reassigned and read after the branch
//      (decodeNibble) must stay ONE in-scope variable, not split into a block-scoped second var.
//   2. declared-type widening: a local first seen as a byte (baload) then reassigned with an int
//      value (unsignedOctet) must be declared int, never byte (a byte cast would truncate 256+b).
//   3. JLS 5.6.2 binary numeric promotion: byte/short/char arithmetic yields int (sumBytes, hex).
//   4. array-element store narrowing: an int value stored into a byte[] must re-emit the (byte) cast
//      that bastore performs implicitly (xorAll).
// The class compiles to bytecode with NO LocalVariableTable (javac default), so the decompiler must
// reconstruct every local type by inference — exactly the production-jar scenario these bugs hit.
public class IntCategoryNarrowing {
	// Slot-merge: result is initialized once and conditionally reassigned in two arms, then read
	// after the if. The decompiler must keep a single `int result` whose scope dominates the return.
	static int decodeNibble(int c) {
		int result = -1;
		if (c >= 48 && c <= 57) {
			result = c - 48;
		} else if (c >= 97 && c <= 102) {
			result = c - 87;
		}
		return result;
	}

	// Declared-type widening (non-loop): o is byte-shaped from baload but must be declared int so
	// `o = 256 + o` is a legal, non-truncating store (the unsigned-octet idiom).
	static int unsignedOctet(byte[] data, int i) {
		int o = data[i];
		if (o < 0) {
			o = 256 + o;
		}
		return o;
	}

	// Binary numeric promotion + array-store narrowing: x is int (the &0xff result); the store into a
	// byte[] must re-emit the (byte) cast bastore performs implicitly.
	static byte[] xorAll(byte[] src, int key) {
		byte[] dst = new byte[src.length];
		for (int i = 0; i < src.length; i++) {
			int x = (src[i] ^ key) & 0xff;
			dst[i] = (byte) (x + 1);
		}
		return dst;
	}

	// Byte arithmetic chained through an int accumulator (promotion must yield int, not byte).
	static int sumBytes(byte[] data) {
		int acc = 0;
		for (int i = 0; i < data.length; i++) {
			byte b = data[i];
			acc += (b & 0xff) * 31 + 7;
		}
		return acc;
	}

	static String hex(byte[] b) {
		StringBuilder sb = new StringBuilder();
		for (int i = 0; i < b.length; i++) {
			int v = b[i] & 0xff;
			sb.append(Character.forDigit((v >> 4) & 0xf, 16));
			sb.append(Character.forDigit(v & 0xf, 16));
		}
		return sb.toString();
	}

	public static void main(String[] args) {
		long acc = 1469598103934665603L;
		byte[] data = new byte[256];
		for (int i = 0; i < 256; i++) {
			data[i] = (byte) (i * 37 + 11);
		}
		for (int c = 0; c < 128; c++) {
			acc = acc * 131 + decodeNibble(c) + 1000;
		}
		for (int i = 0; i < 256; i++) {
			acc = acc * 131 + unsignedOctet(data, i);
		}
		byte[] x = xorAll(data, 0x5a);
		acc = acc * 1315423911L + sumBytes(x);
		System.out.println("fp=" + acc + " hex=" + hex(x));
	}
}

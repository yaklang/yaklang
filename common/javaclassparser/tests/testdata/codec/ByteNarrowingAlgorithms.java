package codec;

// ByteNarrowingAlgorithms is a self-hosted regression battery for narrowing-conversion rendering.
// Java requires an explicit cast when an int-typed expression is stored back into a byte/short/char
// SIMPLE assignment (`b = b + 256` is illegal; `b = (byte)(b + 256)` is required). In bytecode the
// cast is an i2b/i2c/i2s opcode that the decompiler must re-materialize as an explicit `(byte)` cast.
// Dropping it produces "incompatible types: possible lossy conversion from int to byte" and breaks
// recompilation. Compound assignments (`b += x`) carry an implicit narrowing and must stay compound.
//
// Keywords: narrowing conversion, i2b, i2c, i2s, byte assignment cast, lossy conversion.
public class ByteNarrowingAlgorithms {
    static byte byteSimple(byte b, int k) {
        b = (byte) (b + k);          // iadd; i2b; bstore  -> must render the (byte) cast
        b = (byte) (b * 3 + 256);    // imul/iadd; i2b
        b = (byte) (b ^ 0x5A);       // ixor; i2b
        b = (byte) (~b);
        return b;
    }

    static byte byteCompound(byte b, int k) {
        b += k;                      // implicit i2b -> must stay `b += k`
        b *= 7;
        b ^= 0xFF;
        b <<= 2;
        return b;
    }

    static short shortSimple(short s, int k) {
        s = (short) (s + k);         // i2s
        s = (short) (s * 31 + 70000);
        return s;
    }

    static char charSimple(char c, int k) {
        c = (char) (c + k);          // i2c
        c = (char) (c ^ 0x0F0F);
        return c;
    }

    static byte[] arrayStore(byte[] in) {
        byte[] out = new byte[in.length];
        for (int i = 0; i < in.length; i++) {
            out[i] = (byte) (in[i] + 200);   // bastore after i2b
        }
        return out;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        int[] ks = {0, 1, 7, 200, -300, 65535};
        byte[] bs = {0, 1, 127, -128, -1, 50};
        for (int k : ks) {
            for (byte b : bs) {
                sb.append(byteSimple(b, k) & 0xFF).append(',');
                sb.append(byteCompound(b, k) & 0xFF).append(',');
            }
        }
        sb.append('|');
        short[] ss = {0, 1, 32767, -32768, -1, 1000};
        for (int k : ks) {
            for (short s : ss) {
                sb.append(shortSimple(s, k) & 0xFFFF).append(',');
            }
        }
        sb.append('|');
        char[] cs = {0, 'A', 0xFFFF, 0x1234};
        for (int k : ks) {
            for (char c : cs) {
                sb.append((int) charSimple(c, k)).append(',');
            }
        }
        sb.append('|');
        byte[] arr = arrayStore(bs);
        for (byte b : arr) {
            sb.append(b & 0xFF).append(',');
        }
        System.out.println(sb.toString());
    }
}

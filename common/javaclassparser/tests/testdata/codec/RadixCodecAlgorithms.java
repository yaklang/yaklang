package codec;

/**
 * RadixCodecAlgorithms - self-hosted, dependency-free encode/decode PAIRS that each verify by an
 * internal round-trip (decode(encode(x)) == x), so the differential-execution oracle proves the
 * decompiler preserved BOTH directions byte-for-byte. Complements the hash/checksum batteries (which
 * are one-way) with reversible codecs whose decode side stresses control flow the encoders do not.
 *
 * Coverage intent (real Apache-Commons-Codec / Bitcoin shapes):
 *   - hex encode/decode: nibble shifts, char classification if-ladder (hexVal).
 *   - percent (RFC 3986 URL) encode/decode: unreserved-char predicate, `%XX` parsing with a `while`
 *     cursor that advances 1 or 3 (variable-stride loop).
 *   - Base58 (Bitcoin) encode/decode: the big-endian base-256 <-> base-58 long-division with carry
 *     propagation - NESTED loops, `/` and `%`, `>>`/`&` reassembly, post-decrement output cursor
 *     (`out[--pos]`), and leading-zero accounting. This is the densest control-flow exerciser here.
 *   - run-length encode/decode: a run-counting inner `while` with a short-circuit 3-term guard.
 *
 * Every encoded form is folded into the fingerprint as a human-recognizable correctness anchor
 * (hex("Yak")=59616b, base58([0,1,2])=15T, ...). Single public top-level class, static only. Plain
 * counted/cursor loops and ascending if/else only, to stay clear of the CODEC_TODO.md control-flow
 * reconstruction limitations.
 */
public class RadixCodecAlgorithms {

    private static final char[] HEX_LOWER = "0123456789abcdef".toCharArray();
    private static final char[] HEX_UPPER = "0123456789ABCDEF".toCharArray();
    private static final char[] B58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz".toCharArray();

    private static byte[] bytes(String s) {
        byte[] out = new byte[s.length()];
        for (int i = 0; i < s.length(); i++) {
            out[i] = (byte) s.charAt(i);
        }
        return out;
    }

    private static boolean sameBytes(byte[] a, byte[] b) {
        if (a.length != b.length) {
            return false;
        }
        for (int i = 0; i < a.length; i++) {
            if (a[i] != b[i]) {
                return false;
            }
        }
        return true;
    }

    // ---- hex ----
    public static String hexEncode(byte[] data) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < data.length; i++) {
            int v = data[i] & 0xff;
            sb.append(HEX_LOWER[v >>> 4]);
            sb.append(HEX_LOWER[v & 0xf]);
        }
        return sb.toString();
    }

    private static int hexVal(char c) {
        if (c >= '0' && c <= '9') {
            return c - '0';
        }
        if (c >= 'a' && c <= 'f') {
            return (c - 'a') + 10;
        }
        if (c >= 'A' && c <= 'F') {
            return (c - 'A') + 10;
        }
        return -1;
    }

    public static byte[] hexDecode(String s) {
        byte[] out = new byte[s.length() / 2];
        for (int i = 0; i < out.length; i++) {
            int hi = hexVal(s.charAt(2 * i));
            int lo = hexVal(s.charAt((2 * i) + 1));
            out[i] = (byte) ((hi << 4) | lo);
        }
        return out;
    }

    // ---- percent / RFC 3986 URL ----
    public static String percentEncode(byte[] data) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < data.length; i++) {
            int c = data[i] & 0xff;
            boolean unreserved = ((c >= 'A') && (c <= 'Z'))
                    || ((c >= 'a') && (c <= 'z'))
                    || ((c >= '0') && (c <= '9'))
                    || (c == '-') || (c == '_') || (c == '.') || (c == '~');
            if (unreserved) {
                sb.append((char) c);
            } else {
                sb.append('%');
                sb.append(HEX_UPPER[c >>> 4]);
                sb.append(HEX_UPPER[c & 0xf]);
            }
        }
        return sb.toString();
    }

    public static byte[] percentDecode(String s) {
        byte[] out = new byte[s.length()];
        int outPos = 0;
        int i = 0;
        while (i < s.length()) {
            char c = s.charAt(i);
            if (c == '%') {
                int hi = hexVal(s.charAt(i + 1));
                int lo = hexVal(s.charAt(i + 2));
                out[outPos] = (byte) ((hi << 4) | lo);
                outPos++;
                i += 3;
            } else {
                out[outPos] = (byte) c;
                outPos++;
                i++;
            }
        }
        byte[] result = new byte[outPos];
        for (int k = 0; k < outPos; k++) {
            result[k] = out[k];
        }
        return result;
    }

    // ---- Base58 (Bitcoin) ----
    public static String base58Encode(byte[] input) {
        if (input.length == 0) {
            return "";
        }
        int zeros = 0;
        while ((zeros < input.length) && (input[zeros] == 0)) {
            zeros++;
        }
        byte[] in = new byte[input.length];
        for (int i = 0; i < input.length; i++) {
            in[i] = input[i];
        }
        char[] out = new char[input.length * 2];
        int outPos = out.length;
        int start = zeros;
        while (start < in.length) {
            int rem = 0;
            for (int i = start; i < in.length; i++) {
                int num = (rem << 8) | (in[i] & 0xff);
                in[i] = (byte) (num / 58);
                rem = num % 58;
            }
            outPos--;
            out[outPos] = B58[rem];
            if (in[start] == 0) {
                start++;
            }
        }
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < zeros; i++) {
            sb.append('1');
        }
        for (int i = outPos; i < out.length; i++) {
            sb.append(out[i]);
        }
        return sb.toString();
    }

    private static int base58Index(char c) {
        for (int i = 0; i < B58.length; i++) {
            if (B58[i] == c) {
                return i;
            }
        }
        return -1;
    }

    public static byte[] base58Decode(String s) {
        if (s.length() == 0) {
            return new byte[0];
        }
        int zeros = 0;
        while ((zeros < s.length()) && (s.charAt(zeros) == '1')) {
            zeros++;
        }
        // Multiply-accumulate (Bitcoin reference) base58 -> base256: each base58 digit is folded into a
        // little-endian base256 accumulator with carry propagation. Unlike repeated base256 division of
        // a base58-limb number, this emits exactly one byte per overflow so there are no spurious
        // leading zero bytes.
        byte[] buf = new byte[s.length()];
        int bufLen = 0;
        for (int i = zeros; i < s.length(); i++) {
            int carry = base58Index(s.charAt(i));
            for (int j = 0; j < bufLen; j++) {
                carry += 58 * (buf[j] & 0xff);
                buf[j] = (byte) (carry & 0xff);
                carry >>= 8;
            }
            while (carry > 0) {
                buf[bufLen] = (byte) (carry & 0xff);
                bufLen++;
                carry >>= 8;
            }
        }
        byte[] result = new byte[zeros + bufLen];
        for (int i = 0; i < bufLen; i++) {
            result[zeros + i] = buf[(bufLen - 1) - i];
        }
        return result;
    }

    // ---- run-length ----
    public static String rleEncode(String s) {
        StringBuilder sb = new StringBuilder();
        int i = 0;
        while (i < s.length()) {
            char c = s.charAt(i);
            int count = 1;
            while (((i + count) < s.length()) && (s.charAt(i + count) == c) && (count < 9)) {
                count++;
            }
            sb.append((char) ('0' + count));
            sb.append(c);
            i += count;
        }
        return sb.toString();
    }

    public static String rleDecode(String s) {
        StringBuilder sb = new StringBuilder();
        int i = 0;
        while (i < s.length()) {
            int count = s.charAt(i) - '0';
            char c = s.charAt(i + 1);
            for (int k = 0; k < count; k++) {
                sb.append(c);
            }
            i += 2;
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        String[] texts = {"", "Yak", "hello, world!", "AAAabbbbbZ", "https://a.b/c?d=e&f=g~h"};

        // hex round-trip
        for (int t = 0; t < texts.length; t++) {
            byte[] in = bytes(texts[t]);
            String enc = hexEncode(in);
            byte[] dec = hexDecode(enc);
            sb.append(enc).append(sameBytes(in, dec) ? "+" : "!");
        }
        sb.append('/');

        // percent round-trip
        for (int t = 0; t < texts.length; t++) {
            byte[] in = bytes(texts[t]);
            String enc = percentEncode(in);
            byte[] dec = percentDecode(enc);
            sb.append(enc).append(sameBytes(in, dec) ? "+" : "!");
        }
        sb.append('/');

        // base58 round-trip on byte vectors (including leading zeros)
        byte[][] vecs = {
                {},
                {0},
                {0, 0, 1, 2, 3},
                {(byte) 0x00, (byte) 0xff, (byte) 0x80, (byte) 0x10},
                bytes("Bitcoin")
        };
        for (int t = 0; t < vecs.length; t++) {
            String enc = base58Encode(vecs[t]);
            byte[] dec = base58Decode(enc);
            sb.append(enc.length() == 0 ? "_" : enc).append(sameBytes(vecs[t], dec) ? "+" : "!");
        }
        sb.append('/');

        // run-length round-trip
        String[] rleTexts = {"AAAabbbbbZ", "WWWWWWWWWWWW", "abc", ""};
        for (int t = 0; t < rleTexts.length; t++) {
            String enc = rleEncode(rleTexts[t]);
            String dec = rleDecode(enc);
            sb.append(enc).append(dec.equals(rleTexts[t]) ? "+" : "!");
        }

        System.out.println(sb);
    }
}

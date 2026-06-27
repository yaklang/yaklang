package codec;

/**
 * Base32Codec - self-hosted RFC 4648 Base32 (and Base32hex) encode/decode, table-free decode via a
 * linear scan. Canonical RFC 4648 test vectors ("", f, fo, foo, foob, fooba, foobar) are folded into
 * the fingerprint as an explicit correctness anchor (e.g. "foobar" -> "MZXW6YTBOI="), and every input
 * is round-tripped (encode then decode then compare) so a wrong codec shows up immediately.
 *
 * Coverage intent: byte[]/char[]/String iteration, 5-bit grouping with long accumulators, modular and
 * shift arithmetic, switch on the remainder, padding logic. Plain counted loops + ascending if/else.
 * Single public top-level class, static only, deterministic.
 */
public class Base32Codec {

    private static final String ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
    private static final String HEX_ALPHABET = "0123456789ABCDEFGHIJKLMNOPQRSTUV";

    private static byte[] bytes(String s) {
        byte[] out = new byte[s.length()];
        for (int i = 0; i < s.length(); i++) {
            out[i] = (byte) s.charAt(i);
        }
        return out;
    }

    // ---- encode an arbitrary byte[] to Base32 using the given 32-char alphabet, with '=' padding ----
    public static String encode(byte[] data, String alphabet) {
        StringBuilder sb = new StringBuilder();
        int i = 0;
        int len = data.length;
        while (i < len) {
            int chunkLen = len - i;
            if (chunkLen > 5) {
                chunkLen = 5;
            }
            long buffer = 0;
            for (int k = 0; k < 5; k++) {
                buffer <<= 8;
                if (k < chunkLen) {
                    buffer |= (data[i + k] & 0xff);
                }
            }
            // 5 bytes -> 8 base32 symbols (40 bits)
            int outChars;
            switch (chunkLen) {
                case 1: outChars = 2; break;
                case 2: outChars = 4; break;
                case 3: outChars = 5; break;
                case 4: outChars = 7; break;
                default: outChars = 8; break;
            }
            for (int k = 0; k < 8; k++) {
                int shift = 35 - 5 * k;
                int idx = (int) ((buffer >>> shift) & 0x1f);
                if (k < outChars) {
                    sb.append(alphabet.charAt(idx));
                } else {
                    sb.append('=');
                }
            }
            i += 5;
        }
        return sb.toString();
    }

    private static int symbolValue(char c, String alphabet) {
        for (int i = 0; i < alphabet.length(); i++) {
            if (alphabet.charAt(i) == c) {
                return i;
            }
        }
        return -1;
    }

    // ---- decode a (possibly padded) Base32 string back to bytes ----
    public static byte[] decode(String encoded, String alphabet) {
        int realLen = 0;
        for (int i = 0; i < encoded.length(); i++) {
            if (encoded.charAt(i) != '=') {
                realLen++;
            }
        }
        // each 8 symbols -> 5 bytes; a partial trailing group of `rem` symbols yields rem*5/8 bytes
        // (rem in {0,2,4,5,7} -> {0,1,2,3,4}). Natural empty-`default` switch form: this is the exact
        // CODEC_TODO.md "Bug K" shape (every case breaks to the same point the empty default falls
        // through to). Now decompiled correctly (SwitchRewriter1 promotes the convergence node to the
        // switch merge and drops the empty default), so the idiom is restored rather than worked around.
        int fullGroups = realLen / 8;
        int rem = realLen % 8;
        int partial = 0;
        switch (rem) {
            case 2: partial = 1; break;
            case 4: partial = 2; break;
            case 5: partial = 3; break;
            case 7: partial = 4; break;
            default: break;
        }
        int outLen = fullGroups * 5 + partial;
        byte[] out = new byte[outLen];
        long buffer = 0;
        int bits = 0;
        int outPos = 0;
        for (int i = 0; i < encoded.length(); i++) {
            char c = encoded.charAt(i);
            if (c == '=') {
                continue;
            }
            int v = symbolValue(c, alphabet);
            if (v < 0) {
                continue;
            }
            buffer = (buffer << 5) | v;
            bits += 5;
            if (bits >= 8) {
                bits -= 8;
                if (outPos < outLen) {
                    out[outPos] = (byte) ((buffer >>> bits) & 0xff);
                    outPos++;
                }
            }
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

    private static String hexOf(byte[] data) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < data.length; i++) {
            int v = data[i] & 0xff;
            sb.append(Character.forDigit(v >>> 4, 16));
            sb.append(Character.forDigit(v & 0xf, 16));
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        String[] inputs = {"", "f", "fo", "foo", "foob", "fooba", "foobar", "Hello, Base32!", "\u0000\u00ff\u0010"};
        for (int k = 0; k < inputs.length; k++) {
            byte[] d = bytes(inputs[k]);

            String std = encode(d, ALPHABET);
            String hex = encode(d, HEX_ALPHABET);
            sb.append(std).append(';');
            sb.append(hex).append(';');

            // round-trip both alphabets and assert (folded as 1/0 into the fingerprint)
            byte[] backStd = decode(std, ALPHABET);
            byte[] backHex = decode(hex, HEX_ALPHABET);
            sb.append(sameBytes(d, backStd) ? '1' : '0');
            sb.append(sameBytes(d, backHex) ? '1' : '0');
            sb.append(';');
            sb.append(hexOf(backStd)).append('|');
        }
        System.out.println(sb);
    }
}

package codec;

// Self-hosted commons-codec Hex-style battery: strict hex encode/decode with validation. Exercises
// char/byte narrowing (i2c/i2b), nibble bit-shifting (>>> 4, & 0x0F), array allocation/indexing,
// char-range branches, and checked-exception throwing/catching on malformed input - all shapes that
// historically tripped narrowing casts and try-catch reconstruction. Self-verifying via a fingerprint.
public class HexCodecStrict {
    static final char[] DIGITS = {
        '0', '1', '2', '3', '4', '5', '6', '7',
        '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'
    };

    static String encode(byte[] data) {
        char[] out = new char[data.length * 2];
        int j = 0;
        for (int i = 0; i < data.length; i++) {
            int v = data[i] & 0xFF;
            out[j++] = DIGITS[v >>> 4];
            out[j++] = DIGITS[v & 0x0F];
        }
        return new String(out);
    }

    static int nibble(char c) {
        if (c >= '0' && c <= '9') {
            return c - '0';
        }
        if (c >= 'a' && c <= 'f') {
            return c - 'a' + 10;
        }
        if (c >= 'A' && c <= 'F') {
            return c - 'A' + 10;
        }
        throw new IllegalArgumentException("bad hex char: " + c);
    }

    static byte[] decode(String s) {
        int len = s.length();
        if ((len & 1) != 0) {
            throw new IllegalArgumentException("odd length");
        }
        byte[] out = new byte[len / 2];
        for (int i = 0; i < len; i += 2) {
            int hi = nibble(s.charAt(i));
            int lo = nibble(s.charAt(i + 1));
            out[i / 2] = (byte) ((hi << 4) | lo);
        }
        return out;
    }

    static long fnv(byte[] b) {
        long h = 0xcbf29ce484222325L;
        for (int i = 0; i < b.length; i++) {
            h ^= (b[i] & 0xFF);
            h *= 0x100000001b3L;
        }
        return h;
    }

    public static void main(String[] args) {
        byte[] src = new byte[64];
        for (int i = 0; i < src.length; i++) {
            src[i] = (byte) (i * 7 - 13);
        }
        String hex = encode(src);
        byte[] round = decode(hex);
        long fpSrc = fnv(src);
        long fpRound = fnv(round);

        int oddSeen = 0;
        try {
            decode("abc");
        } catch (IllegalArgumentException e) {
            oddSeen = 1;
        }
        int badSeen = 0;
        try {
            decode("zz");
        } catch (IllegalArgumentException e) {
            badSeen = 1;
        }

        StringBuilder sb = new StringBuilder();
        sb.append("HexCodecStrict:");
        sb.append(hex).append(":");
        sb.append(fpSrc).append(":").append(fpRound).append(":");
        sb.append(fpSrc == fpRound).append(":");
        sb.append(oddSeen).append(":").append(badSeen);
        System.out.println(sb.toString());
    }
}

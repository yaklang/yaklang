package codec;

/**
 * ChecksumAlgorithms - self-hosted, table-free implementations of the classic checksum family, each
 * with its canonical "123456789" check value folded into the fingerprint so the golden line carries a
 * recognizable correctness anchor (CRC16/CCITT-FALSE=0x29b1, CRC16/XMODEM=0x31c3, CRC16/ARC=0xbb3d,
 * CRC32=0xcbf43926). The round-trip oracle then guarantees decompile -> recompile -> run reproduces
 * every value byte-for-byte.
 *
 * Coverage intent: byte[] iteration, masking/narrowing (byte<->int), nested shift/xor loops, unsigned
 * right shift, modular arithmetic. Plain counted loops and ascending if/else only, to avoid the
 * control-flow reconstruction bugs in CODEC_TODO.md. Single public top-level class, static only.
 */
public class ChecksumAlgorithms {

    private static byte[] bytes(String s) {
        byte[] out = new byte[s.length()];
        for (int i = 0; i < s.length(); i++) {
            out[i] = (byte) s.charAt(i);
        }
        return out;
    }

    // ---- CRC16/CCITT-FALSE: poly 0x1021, init 0xffff, no reflection ----
    public static int crc16CcittFalse(byte[] data) {
        int crc = 0xffff;
        for (int i = 0; i < data.length; i++) {
            crc ^= (data[i] & 0xff) << 8;
            for (int b = 0; b < 8; b++) {
                if ((crc & 0x8000) != 0) {
                    crc = (crc << 1) ^ 0x1021;
                } else {
                    crc = crc << 1;
                }
                crc &= 0xffff;
            }
        }
        return crc & 0xffff;
    }

    // ---- CRC16/XMODEM: poly 0x1021, init 0x0000, no reflection ----
    public static int crc16Xmodem(byte[] data) {
        int crc = 0x0000;
        for (int i = 0; i < data.length; i++) {
            crc ^= (data[i] & 0xff) << 8;
            for (int b = 0; b < 8; b++) {
                if ((crc & 0x8000) != 0) {
                    crc = (crc << 1) ^ 0x1021;
                } else {
                    crc = crc << 1;
                }
                crc &= 0xffff;
            }
        }
        return crc & 0xffff;
    }

    // ---- CRC16/ARC (a.k.a. IBM): poly 0xa001 (reflected 0x8005), init 0x0000, reflected ----
    public static int crc16Arc(byte[] data) {
        int crc = 0x0000;
        for (int i = 0; i < data.length; i++) {
            crc ^= (data[i] & 0xff);
            for (int b = 0; b < 8; b++) {
                if ((crc & 1) != 0) {
                    crc = (crc >>> 1) ^ 0xa001;
                } else {
                    crc = crc >>> 1;
                }
            }
        }
        return crc & 0xffff;
    }

    // ---- CRC32 (reflected, poly 0xedb88320), bit-at-a-time ----
    public static long crc32(byte[] data) {
        int crc = 0xffffffff;
        for (int i = 0; i < data.length; i++) {
            crc ^= (data[i] & 0xff);
            for (int b = 0; b < 8; b++) {
                if ((crc & 1) != 0) {
                    crc = (crc >>> 1) ^ 0xedb88320;
                } else {
                    crc = crc >>> 1;
                }
            }
        }
        return (~crc) & 0xffffffffL;
    }

    // ---- Fletcher-16 ----
    public static int fletcher16(byte[] data) {
        int sum1 = 0;
        int sum2 = 0;
        for (int i = 0; i < data.length; i++) {
            sum1 = (sum1 + (data[i] & 0xff)) % 255;
            sum2 = (sum2 + sum1) % 255;
        }
        return (sum2 << 8) | sum1;
    }

    // ---- Fletcher-32 (operates on 16-bit words, big-endian, zero-padded) ----
    public static long fletcher32(byte[] data) {
        long sum1 = 0;
        long sum2 = 0;
        int i = 0;
        int len = data.length;
        while (i + 1 < len) {
            int word = ((data[i] & 0xff) << 8) | (data[i + 1] & 0xff);
            sum1 = (sum1 + word) % 65535;
            sum2 = (sum2 + sum1) % 65535;
            i += 2;
        }
        if (i < len) {
            int word = (data[i] & 0xff) << 8;
            sum1 = (sum1 + word) % 65535;
            sum2 = (sum2 + sum1) % 65535;
        }
        return (sum2 << 16) | sum1;
    }

    // ---- Adler-32 ----
    public static long adler32(byte[] data) {
        long a = 1;
        long b = 0;
        long mod = 65521;
        for (int i = 0; i < data.length; i++) {
            a = (a + (data[i] & 0xff)) % mod;
            b = (b + a) % mod;
        }
        return (b << 16) | a;
    }

    // ---- Internet checksum (RFC 1071, ones-complement 16-bit sum) ----
    public static int internetChecksum(byte[] data) {
        int sum = 0;
        int i = 0;
        int len = data.length;
        while (i + 1 < len) {
            sum += ((data[i] & 0xff) << 8) | (data[i + 1] & 0xff);
            i += 2;
        }
        if (i < len) {
            sum += (data[i] & 0xff) << 8;
        }
        while ((sum >>> 16) != 0) {
            sum = (sum & 0xffff) + (sum >>> 16);
        }
        return (~sum) & 0xffff;
    }

    // ---- BSD checksum (rotate-right accumulate) ----
    public static int bsdChecksum(byte[] data) {
        int checksum = 0;
        for (int i = 0; i < data.length; i++) {
            checksum = (checksum >>> 1) | ((checksum & 1) << 15);
            checksum = (checksum + (data[i] & 0xff)) & 0xffff;
        }
        return checksum;
    }

    // ---- SysV checksum ----
    public static long sysvChecksum(byte[] data) {
        long s = 0;
        for (int i = 0; i < data.length; i++) {
            s += (data[i] & 0xff);
        }
        long r = (s & 0xffff) + ((s & 0xffffffffL) >>> 16);
        return (r & 0xffff) + (r >>> 16);
    }

    private static String h(int v) {
        return Integer.toHexString(v);
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        String[] inputs = {"", "a", "abc", "123456789", "The quick brown fox jumps over the lazy dog", "\u0000\u00ff\u0010\u0080"};
        for (int k = 0; k < inputs.length; k++) {
            byte[] d = bytes(inputs[k]);
            sb.append(h(crc16CcittFalse(d))).append(';');
            sb.append(h(crc16Xmodem(d))).append(';');
            sb.append(h(crc16Arc(d))).append(';');
            sb.append(Long.toHexString(crc32(d))).append(';');
            sb.append(h(fletcher16(d))).append(';');
            sb.append(Long.toHexString(fletcher32(d))).append(';');
            sb.append(Long.toHexString(adler32(d))).append(';');
            sb.append(h(internetChecksum(d))).append(';');
            sb.append(h(bsdChecksum(d))).append(';');
            sb.append(Long.toHexString(sysvChecksum(d))).append('|');
        }

        System.out.println(sb);
    }
}

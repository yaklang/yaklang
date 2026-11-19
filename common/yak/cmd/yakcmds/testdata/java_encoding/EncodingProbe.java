// Fixture for javautil_decompiler_encoding_test.go. Rebuild with:
//   LC_ALL=C.UTF-8 javac -encoding UTF-8 --release 8 -d <outdir> EncodingProbe.java
public class EncodingProbe {
    public static final String CHINESE = "中文字符串常量";
    public static final String JAPANESE = "日本語の文字列";
    public static final String KOREAN = "한국어 문자열";
    public static final String ACCENT = "café naïve © ®";
    public static final String EMOJI = "emoji😀";

    public String greet(String who) {
        return CHINESE + " / " + who;
    }
}

public class Strings {
    public String concat(String a, String b) {
        return a + " " + b + "!";
    }

    public String builder(int n) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < n; i++) {
            sb.append(i).append(',');
        }
        return sb.toString();
    }

    public int countVowels(String s) {
        int c = 0;
        for (int i = 0; i < s.length(); i++) {
            char ch = s.charAt(i);
            if (ch == 'a' || ch == 'e' || ch == 'i' || ch == 'o' || ch == 'u') {
                c++;
            }
        }
        return c;
    }

    public String format(String name, int age) {
        return String.format("%s is %d", name, age);
    }
}

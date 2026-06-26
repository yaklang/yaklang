public class TextBlocks {
    public String json() {
        return """
                {
                    "name": "yak",
                    "value": 42
                }
                """;
    }

    public String query() {
        String table = "users";
        return "SELECT * FROM " + table + " WHERE active = true";
    }
}

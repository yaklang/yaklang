// Source: antlr/grammars-v4 csharp/v8-spec/examples/using_var.cs
using System.IO;
class C {
    void M() {
        using var r = new StringReader("hello");
    }
}

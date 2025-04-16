# Markdown Code Block Extractor

A robust Go package for extracting code blocks from Markdown text. It supports both backtick and tilde delimited code blocks with language specifications.

## Features

- Supports both backtick (```) and tilde (~~~) delimiters
- Handles nested code blocks
- Supports mixed delimiters
- Preserves code block metadata (language, position)
- Handles special characters and edge cases

## Usage

```go
err := markdownextractor.ExtractMarkdownCode(markdown, func(typeName string, code string, startOffset int, endOffset int) {
    fmt.Printf("Type: %s\nCode: %s\nPosition: %d-%d\n", typeName, code, startOffset, endOffset)
})
```

## Examples

### Basic Code Block

    # Title

    ```go
    func main() {
        fmt.Println("Hello")
    }
    ```

### Multiple Code Blocks

    # Multiple Code Blocks

    ```python
    print("Hello")
    ```

    Some text

    ```javascript
    console.log("World")
    ```

### Empty Type Code Block

    ```
    plain text
    ```

### Nested Code Blocks

    ```markdown
    `code` inside code block
    ```

### Code Block with Spaces

    ```  go  
    func main() {}
    ```

### Code Block with Special Characters

    ```go
    func main() {
        // 中文注释
        fmt.Println("特殊字符: ~!@#$%^&*()_+")
    }
    ```

### Code Block with Line Breaks in Type

    ```go
    python
    func main() {}
    ```

### Empty Code Block

    ```go

    ```

### Code Block with Only Spaces

    ```go
        
    ```

### Code Block with Backticks in Content

    ```go
    fmt.Println("```")
    ```

### Mixed Delimiters

    ```markdown
    ~~~python
    print("Nested with different delimiters")
    ~~~
    ```

### Malformed Backticks

    ```go
    func main() {
        fmt.Println("Hello")
    }
    ``

## Error Handling

The package returns `ErrUnclosedCodeBlock` when it encounters an unclosed code block.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. 
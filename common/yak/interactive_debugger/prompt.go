package debugger

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Prompt struct {
	prefix string
	reader bufio.Reader
}

func NewPrompt(prefix string) *Prompt {
	return &Prompt{
		prefix: prefix,
		reader: *bufio.NewReader(os.Stdin),
	}
}

func (p *Prompt) Input() (string, error) {
	fmt.Printf("%s ", p.prefix)
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	return input, nil
}

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: helper <mode> [arg]")
		os.Exit(1)
	}
	mode := os.Args[1]

	switch mode {
	case "exit-immediately":
		os.Exit(0)

	case "echo-stdout":
		msg := "default"
		if len(os.Args) > 2 {
			msg = os.Args[2]
		}
		fmt.Println(msg)

	case "echo-stderr":
		msg := "default"
		if len(os.Args) > 2 {
			msg = os.Args[2]
		}
		fmt.Fprintln(os.Stderr, msg)

	case "cat-stdin":
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			fmt.Println(scanner.Text())
		}

	case "run-forever":
		// Keep running until killed
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		time.Sleep(50 * time.Millisecond) // simulate cleanup
		os.Exit(1)

	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		os.Exit(1)
	}
}

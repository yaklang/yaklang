package main

import "C"
import "fmt"

func main() {}

//export yak_internal_print_int
func yak_internal_print_int(n int64) {
	fmt.Println(n)
}

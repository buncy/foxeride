package main

import "fmt"

func main() {

	result := add(5, 6)
	fmt.Println(result)
}

func add(a int, b int) int {
	return a + b
}

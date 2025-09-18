package main

import (
	"fmt"
)

func main() {
	ch, ch2 := make(chan int), make(chan int)
	go func(i int) {
		a := 0
		for j := 0; j < i; j++ {
			a += <-ch
			fmt.Printf("a = %d\n", a)
		}
		ch2 <- a
	}(100)
	go func(i int) {
		for j := 0; j < i; j++ {
			ch <- j
		}
	}(100) // Passes argument 1000 to the function literal.
	v := <-ch2
	fmt.Printf("Total %d\n", v)
}

package main
import (
	"fmt";
)
func main() {
	a := make(chan int, 20);
	end := make(chan int, 1);
	go func() {
		for i := 1; i< 20; i++ {
			_ = a <- i;
		}
	}();
	go func() {
		for i := 1; i< 20; i++ {
			fmt.Printf("%d\n", <-a);
		}
		end <- 1;
	}();
	<-end;
}

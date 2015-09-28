package main
import "fmt"
var ch, ch2, end = make(chan int), make(chan int), make(chan bool,1);
type path struct { vpath chan int; end chan bool; }
func main() {
	go func() {
		L: for {
			select {
			case ch <- 1:
			case ok := <- end:
				end <- ok;
				break L;
			}
		}
		ch2 <- 1;
	}();
	go func() {
		L: for {
			select {
			case n := <- ch:
				fmt.Printf("recv %d\n", n);
				ch2 <- 1;
			case ok := <- end:
				end <- ok;
				break L;
			}
		}
	}();
	<-ch2;
	end <- true;
	fmt.Printf("%d\n", <-ch2);
}
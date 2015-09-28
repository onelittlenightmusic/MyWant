package main
import (
	"fmt";
	"runtime"
)
var ch, ch2, ch3 = make(chan int), make(chan int), make(chan int);
func main() {
	go func() {
		ch <-1;
		<-ch;
		fmt.Printf("Thread End\n");
		ch <-1;
	}();
	go func() {
		ch2 <-1;
		runtime.Goexit();
	}();
	<-ch;
	<-ch2;
	ch<-1;
	<-ch;
	fmt.Printf("End\n");
}

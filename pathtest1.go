package main
import (
	"fmt";
	"runtime"
)
var ch, ch2, end = make(chan int), make(chan int), make(chan bool,1);
type path struct { vpath chan int; end chan bool; }
func recv(a path) int {
	select {
	case n := <- a.vpath:
		return n;
	case <- a.end:
		fmt.Printf("kill recv\n");
		<-ch2;
		runtime.Goexit();
	}
	return 0;
}
func send(b path, n int) {
	select {
	case b.vpath <- n:
		return;
	case <- b.end:
		fmt.Printf("kill send\n");
		ch <- 0;
		runtime.Goexit();
	}
	return;
}
var p = path {
	make(chan int),
	make(chan bool, 1)
};
func main() {
	go func() {
		i := 0;
		for {
			send(p, i);
			fmt.Printf("send %d finished\n", i);
			i++;
			ch2 <- 1;
		}
	}();
	go func() {
		for {
			n := recv(p);
			fmt.Printf("recv %d finished\n", n);
		}
	}();
	<-ch2;
	fmt.Printf("close endpath\n");
	close(p.end);
	fmt.Printf("%d\n", <-ch);
}
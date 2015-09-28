package main
import (
	"fmt";
//	"runtime"
)
type packet struct {
	i int;
	end bool;
}
var ch= make(chan packet, 1);
func main() {
	go func() {
		ch <- packet{1,true};
	}();
	a:=<-ch;
	if a.end { fmt.Printf("%d\n", a.i); }
}

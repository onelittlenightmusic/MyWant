package main
import( "fmt" )
func ch_make () chan float {
	return make(chan float);
}
func main() {
	out := ch_make();
	ch_async := make(chan float, 1);
	go func() {
		for {
			select {
			case n := <- ch_async:
				fmt.Printf("in %f\n", n);
			case out <- 0:
				break;
			}
		}
	}();
/*		go func() {
			if ch_async <- 3 { fmt.Printf("sendOK\n") } else { fmt.Printf("sendNG\n") }
		}();
		if _, ok := <- ch_async; ok { fmt.Printf("recvOK\n") } else { fmt.Printf("recvNG\n") }
//		ch:= ch_make();
		ch := make(chan float, 1);
		ch2 := make(chan float, 1);
		ch4 := make(chan float, 10);
		chlist2 := []chan float{ ch, ch2 };
//		var chlist [2]chan float = ;
//		chlist[0] = ch;
//		chlist[1] = ch2;
		ch3 := chlist2[1];
		ch3 <- 2;
		ch3 = chlist2[0];
		ch3 <-2;
		ch4 <- 1.0;
		fmt.Printf("%d\n", len(ch4));
		<-ch4;
		out2:= make(chan int);
		go func() { <- ch2; <- ch; out2 <- 2 }();
		go func() { <-out2; out <- 2 }();
//		out <- 2;
	}();
*/
	ch_async <- 1;
	<- out;
	fmt.Printf("end\n");
}
	
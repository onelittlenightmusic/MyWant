package main
import (
	"fmt";
)


type tupple struct {
	num int;
	rownum int;
	row [32]int;
}
func double(in, out chan tupple) {
	n := 0;
	for {
		select {
		case x := <- in:
			if x.num<0 { out <- x; break; }
			x.rownum++;
			x.num = n;
			n++;
			x.row[x.rownum-1] = 0;
			out <- x;
			x.num = n;
			n++;
			x.row[x.rownum-1] = 1;
			out <- x;
		}
	}
};
func plus(in, out chan tupple) {
//	n := 0;
	for {
		select {
		case x := <- in:
			if x.num<0 { out <- x; break;}
			_row_max := x.rownum -1;
			for i := 0; i < _row_max; i++ {
				x.rownum++;
				x.row[x.rownum-1] = (x.row[_row_max] + x.row[i])%2;
			}
			out <- x;
		}
	}
};
func tested(p [32]int) bool {
	if p[2]>0 && p[4]>0 {
		panic(fmt.Sprintf("%v, %v", p[2], p[4]));
		return false;
	}
	return true;
};
func main() {
	defer func() { fmt.Printf("defered main\n"); }();
	a := make(chan tupple, 32);
	b := make(chan tupple, 32);
	c := make(chan tupple, 32);
	d := make(chan tupple, 32);
	end := make(chan int, 1);
	//double
	_loop_max := 3;
	go double(a, c);
	go plus(c, b);
//	go double(c, b);
	go func() {
		loop := 0;
		for {
			select {
			case x := <- b:
				switch {
				case loop==_loop_max:
					end <- 1;
				case x.num<0:
					loop++;
					a <- x;
				case loop>= _loop_max -1:
					for i:=0; i<x.rownum; i++ {
						fmt.Printf("%d\t", x.row[i]);
					}
					fmt.Printf("\n");
					d <- x;
				default:
					a <- x;
				}
			}
		}
	}();
// call tested function
	go func() {
		for {
			select {
			case x := <- d:
				func() {
					defer func() {
						if r:=recover(); r!=nil { fmt.Printf("recovered tested\n"); }
					}();
					tested(x.row);
				}();
			}
		}
	}();
	a <- tupple{ 0, 1, [32]int{0} };
	a <- tupple{ 1, 1, [32]int{1} };
	a <- tupple{ -1, 1, [32]int{0} };
	<-end;
}


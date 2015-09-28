package main
import "fmt"

type tupple struct {
	num int;
	rownum int;
	row [32]int;
}

func chain() (in chan tupple, add_func func(func(tupple) tupple) chan tupple) {
	in = make(chan tupple, 1)
	add_func = func(f func(tupple) tupple) (cout chan tupple) {
		cout = make(chan tupple, 1)
		go func(ch1, ch2 chan tupple) {
			for {
				p := <- ch1
				for i:=0; i<x.rownum; i++ {
					fmt.Printf("%d\t", x.row[i]);
				}
				fmt.Printf("\n");
				ch2 <- f(p)
			}
		}(in, cout)	
		in = cout
		return
	}
	return
}

func double(x tupple) tupple {
			if x.num<0 { return x; }
			x.rownum++
			x.num = n
			n++
			x.row[x.rownum-1] = 0
			_ =  out <- x
			x.num = n
			n++
			x.row[x.rownum-1] = 1
			_ = out <- x
}

func plus(x tupple) tupple {
			if x.num<0 { return x; }
			_row_max := x.rownum -1
			for i := 0; i < _row_max; i++ {
				x.rownum++
				x.row[x.rownum-1] = (x.row[_row_max] + x.row[i])%2
			}
			return x
}

func main() {
	cstart, add_chain := chain()
	add_chain(plus);
	cend := add_chain(double);
	// Function calls are evaluated left-to-right.
	cstart <- 1
	println(<- cend)
	cstart <- 100
	println(<- cend)
}


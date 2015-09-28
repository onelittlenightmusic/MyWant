package main
import "fmt"

type tupple struct {
	num int;
	rownum int;
	row [32]int;
}

func chain() (in chan tupple, add_func func(func(tupple) tupple) chan tupple) {
	in = make(chan tupple, 10)
	add_func = func(f func(tupple) tupple) (cout chan tupple) {
		cout = make(chan tupple, 10)
		go func(ch1, ch2 chan tupple) {
			for {
				p := <- ch1
/*				for i:=0; i<p.rownum; i++ {
					fmt.Printf("%d\t", p.row[i]);
				}
				fmt.Printf("\n");
*/
				ch2 <- f(p)
			}
		}(in, cout)	
		in = cout
		return
	}
	return
}

func double(in chan tupple) (out chan tupple) {
			x := <- in
			if x.num<0 { out <- x; return }
			x.rownum++
			x.num = 1
			x.row[x.rownum-1] = 0
			out <- x
			return
}

func plus(in chan tupple) (out chan tupple) {
			x := <- in
			if x.num<0 { out <- x; return }
			_row_max := x.rownum -1
			for i := 0; i < _row_max; i++ {
				x.rownum++
				x.row[x.rownum-1] = (x.row[_row_max] + x.row[i])%2
			}
			out <- x
			return
}

func main() {
	cstart, add_chain := chain()
	add_chain(double);
	cend := add_chain(plus);
	cstart <- tupple{ 0, 1, [32]int{0} }
	cstart <- tupple{ 1, 1, [32]int{1} }
	cstart <- tupple{ -1, 1, [32]int{0} }
			for {
				p := <- cend
				if p.num < 0 { break }
				for i:=0; i<p.rownum; i++ {
					fmt.Printf("%d\t", p.row[i]);
				}
				fmt.Printf("\n");
			}
}



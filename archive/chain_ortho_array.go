package main

import (
	"fmt"
	"mywant/src/chain"
)

type tupple struct {
	Num    int
	Rownum int
	Row    [32]int
}

//type tupple chain.Tupple

func init_func(in chain.Chan) (fin bool) {
	t0, t1, te := tupple{0, 1, [32]int{0}},
		tupple{1, 1, [32]int{0}},
		tupple{-1, 1, [32]int{0}}
	in <- t0
	in <- t1
	in <- te
	fin = true
	return
}
func double(in, out chain.Chan) (fin bool) {
	x := (<-in).(tupple)
	if x.Num < 0 {
		out <- x
		fin = true
		return
	}
	x.Rownum++
	x.Num = 1
	x.Row[x.Rownum-1] = 0
	out <- x
	x.Row[x.Rownum-1] = 1
	out <- x
	fin = false
	return
}

func plus(in, out chain.Chan) (fin bool) {
	x := (<-in).(tupple)
	if x.Num < 0 {
		out <- x
		fin = true
		return
	}
	_row_max := x.Rownum - 1
	for i := 0; i < _row_max; i++ {
		x.Rownum++
		x.Row[x.Rownum-1] = (x.Row[_row_max] + x.Row[i]) % 2
	}
	out <- x
	fin = false
	return
}

func end_func(cend chain.Chan) (fin bool) {
	p := (<-cend).(tupple)
	if p.Num < 0 {
		fin = true
		return
	}
	for i := 0; i < p.Rownum; i++ {
		fmt.Printf("%d\t", p.Row[i])
	}
	fmt.Printf("\n")
	fin = false
	return
}

func main() {
	/*	start_chain, add_chain, end_chain := chain.Chain()
			start_chain	(init_func)
			add_chain	(double)
			add_chain	(plus)
		//	end_chain	(end_func)
			add_chain	(double)
			add_chain	(plus)
			end_chain	(end_func)
	*/
	var c1 chain.C_chain
	c1.Start(init_func)
	c1.Add(double)
	c1.Add(plus)
	c1.Add(double)
	c1.Add(plus)
	c1.End(end_func)
	chain.Run()
}

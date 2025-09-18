package main

type Tupple struct {
	Num    int
	Rownum int
	Row    [32]int
}

func Chain() (start_func func(func(chan Tupple)),
	add_func func(func(chan Tupple, chan Tupple)),
	end_func func(func(chan Tupple))) {
	in := make(chan Tupple)
	cstart := in
	start_func = func(f func(chan Tupple)) {
		if in == nil {
			return
		}
		go f(cstart)
	}
	add_func = func(f func(chan Tupple, chan Tupple)) {
		if in == nil {
			return
		}
		cout := make(chan Tupple, 10)
		go func(ch1, ch2 chan Tupple) {
			for {
				f(ch1, ch2)
			}
		}(in, cout)
		in = cout
		return
	}
	end_func = func(f func(chan Tupple)) {
		if in == nil {
			return
		}
		f(in)
		in = nil
	}
	return
}

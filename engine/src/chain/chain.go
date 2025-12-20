package chain

/*
type Tuple struct {
	Num int
	Rownum int
	Row[32]int
}
*/
//Tuple Interface is the object type which Chain module can deal with
type Tuple any

// Chan is the channel type which Chain module can deal with
type Chan chan Tuple

// Initializing variances which changed in calling function Chain()
var (
	ch_start   chan int = make(chan int, 1)
	ch_end     chan int = make(chan int, 1)
	init_start chan int = ch_start
	init_end   chan int = ch_end
)

// next_start() is called only by Add
func next_start() func() {
	next_ch_start, prev_ch_start := make(chan int, 1), ch_start
	ch_start = next_ch_start
	return func() {
		next_ch_start <- <-prev_ch_start
	}
}

// next_end() is called only by End function
func next_end() func() {
	next_ch_end, prev_ch_end := make(chan int, 1), ch_end
	ch_end = next_ch_end
	return func() {
		next_ch_end <- <-prev_ch_end
	}
}

type C_chain struct {
	// start, end func(func(Chan)(bool)) add func(func(_, _ Chan)(bool))
	In, Ch_start chan Tuple
}

func (c *C_chain) Start(f func(Chan) bool) {
	if c.In != nil {
		return
	}
	start := next_start()
	c.In = make(chan Tuple, 10)
	c.Ch_start = c.In
	go func(ch1 Chan) {
		start()
		for !f(ch1) {
		}
	}(c.Ch_start)
}
func (c *C_chain) Add(f func(Chan, Chan) bool) {
	if c.In == nil {
		//Start
		start := next_start()
		c.In = make(chan Tuple, 10)
		c.Ch_start = c.In
		go func(ch1 Chan) {
			start()
			for !f(nil, ch1) {
			}
		}(c.Ch_start)
	} else {
		prev_in, cout := c.In, make(Chan, 10)
		c.In = cout
		go func(ch1, ch2 Chan) {
			for !f(ch1, ch2) {
			}
		}(prev_in, cout)
	}
	return
}
func (c *C_chain) End(f func(Chan) bool) {
	if c.In == nil {
		return
	}
	end := next_end()
	prev_in := c.In
	c.In = nil
	go func(ch1 Chan) {
		for !f(ch1) {
		}
		end()
	}(prev_in)
	return
}

// Chain() is main function of this module
func Chain() (start_func func(func(Chan) bool),
	add_func func(func(_, _ Chan) bool),
	end_func func(func(Chan) bool),
	get_chan func() Chan) {
	var in, cstart chan Tuple
	in = nil
	//	defer close(in)
	start_func = func(f func(Chan) bool) {
		if in != nil {
			return
		}
		start := next_start()
		in = make(chan Tuple, 10)
		cstart = in
		go func(ch1 Chan) {
			start()
			for !f(ch1) {
			}
		}(cstart)
	}
	add_func = func(f func(Chan, Chan) bool) {
		if in == nil {
			return
		}
		cout := make(Chan, 10)
		go func(ch1, ch2 Chan) {
			for !f(ch1, ch2) {
			}
		}(in, cout)
		in = cout
		return
	}
	end_func = func(f func(Chan) bool) {
		if in == nil {
			return
		}
		end := next_end()
		go func(ch1 Chan) {
			for !f(ch1) {
			}
			end()
		}(in)
		//		for !f(in) { }
		in = nil
		return
	}
	get_chan = func() Chan {
		return in
	}
	return
}

func Merge(c1, c2 Chan) {
	go func(a1, a2 Chan) {
		for {
			a1 <- <-a2
		}
	}(c1, c2)
	return
}
func default_merge(a, b, c Chan) bool {
	select {
	case x := <-a:
		c <- x
	case x := <-b:
		c <- x
	}
	return false
}
func default_split(a, b, c Chan) bool {
	x := <-a
	b <- x
	c <- x
	return false
}

func (c1 *C_chain) Merge(c2 C_chain, f func(_, _, _ Chan) bool) {
	if f == nil {
		f = default_merge
	}
	c2_last := c2.In
	c1.Add(func(a, cout Chan) bool { return f(a, c2_last, cout) })
	c2.In = nil
	return
}

func (c1 *C_chain) Split(f func(_, _, _ Chan) bool) (c2 C_chain) {
	if f == nil {
		f = default_split
	}
	c2 = C_chain{nil, nil}
	c2.Start(func(a Chan) bool { return true })
	c1.Add(func(a, b Chan) bool { return f(a, b, c2.Ch_start) })
	return
}

func Run() {
	init_start <- 0
	init_end <- 0
	<-ch_end
}

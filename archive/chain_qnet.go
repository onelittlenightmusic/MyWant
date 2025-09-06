package main
import (
	"fmt"
	"MyWant/chain"
	"math/rand"
//	"runtime"
)

type QueuePacketTuple struct {
	Num int
	Time float64
}

func (t *QueuePacketTuple) isEnded() bool {
	return t.Num < 0
}

type xfunc func(in, out chain.Chan) bool

//type QueuePacketTuple chain.Tupple
func queue(t_ave float64) (xfunc) {
	t_buf, t_sum := 0.0, 0.0
	n_buf := 0
	return func(in, out chain.Chan) bool {
		t := (<- in).(QueuePacketTuple)
		if t.isEnded() {
			fmt.Printf("[QUEUE STAT] %f [AVERAGE] %f\n", t_ave, t_sum/float64(n_buf))
			out <- t
			return true
		}
		if t.Time > t_buf { t_buf = t.Time }
		t_buf += t_ave*float64(rand.ExpFloat64())
		out <- QueuePacketTuple{ t.Num, t_buf }
		t_sum += t_buf - t.Time
		n_buf = t.Num
		return false
	}
}
func filter() (xfunc) {
	p := 0.5
	return func(in, out chain.Chan) bool {
		if p >= rand.Float64() { out <- <- in }
		return true
	}
}

func init_func(d float64, i int) (xfunc) {
	t, j := 0.0, 0
	return func(_, in chain.Chan) bool {
		if j++; j >= i {
			in <- QueuePacketTuple{ -1, 0 }
			fmt.Printf("[END] generate\n")
			return true
		}
		t += d*float64(rand.ExpFloat64())
		in <- QueuePacketTuple{ j, t }
		return false
	}
}

func combine () (func(_, _, _ chain.Chan) (bool)) {
	i := 0
	t_buf := 0.0
	close_either := false
	var in_last, in_other chain.Chan = nil, nil
	return func(in, in2, out chain.Chan) bool {
		if in_last == nil { in_last, in_other = in, in2 }
		t := (<- in_last).(QueuePacketTuple)
		if t.isEnded() {
			if close_either {
				out <- t
				return true
			} else {
				close_either = true
				in_last, in_other = in_other, in_last
				return false
			}
		}
		if !close_either && t.Time > t_buf {
			t.Time, t_buf = t_buf, t.Time
			in_last, in_other = in_other, in_last
		}
		t.Num = i
		out <- t
		i++
		return false
	}
}

func end_func (cend chain.Chan) bool {
	t := (<- cend).(QueuePacketTuple)
	return t.Num < 0
}
	
func main() {
/*
	start_chain, add_chain, end_chain, get := chain.Chain()
	start_chain	(init_func(3.0, 1000))
	add_chain	(queue(0.5))
	add_chain	(queue(0.9))
	start_chain2, add_chain2, _, get2 := chain.Chain()
	start_chain2	(init_func(3.0, 1000))
	add_chain2	(queue(0.7))
	chain.Merge	(get(), get2())
	add_chain	(queue(0.3))
	end_chain	(end_func)
//	end_chain2	(end_func)
*/
	var c, c2 chain.C_chain
	c.Add(init_func(3.0, 1000))
	c.Add	(queue(0.5))
	c.Add	(queue(0.9))

	c2.Add(init_func(4.0, 1000))
	c2.Add	(queue(0.7))
	c.Merge	(c2, combine())
	c.Add	(queue(0.01))
	c3 := c.Split(nil)
	c.Add	(queue(0.3))
	c3.Add	(queue(0.5))
	c3.End	(end_func)
	c.End	(end_func)
//	c2.Add	(queue(0.3))
	chain.Run()
}


package main
import (
	"fmt"
	"./chain"
	"math/rand"
//	"runtime"
)

type tupple struct {
	Num int
	Time float64
}
//type tupple chain.Tupple
func queue(t_ave float64) (func(_, _ chain.Chan)(bool)) {
	t_buf, t_sum := 0.0, 0.0
	n_buf := 0
	return func(in_t, out_t chain.Chan) (fin bool) {
		t := (<- in_t).(tupple)
		if t.Num < 0 {
			fmt.Printf("[QUEUE STAT] %f [AVERAGE] %f\n", t_ave, t_sum/float64(n_buf))
			out_t <- t
			fin = true 
			return
		}
		if t.Time > t_buf { t_buf = t.Time }
		t_buf += t_ave*float64(rand.ExpFloat64())
		out_t <- tupple{ t.Num, t_buf }
		t_sum += t_buf - t.Time
		n_buf = t.Num
		fin = false
		return
	}
}
func filter() (func(_, _ chain.Chan)) {
	p := 0.5
	return func(in_t, out_t chain.Chan) {
		if p >= rand.Float64() { out_t <- <- in_t }
	}
}
/*
func async(in chain.Chan) (path) {
	out := newasyncpath(n)
	return out
}

func couple(in, out chain.Chan) {
	ain := async(in,100)
	go func() {
		t, a_buf := 0.0, 0.0
		recv_cnt:= 0
		L1 : for {
			a, b := <- ain.vpath
			if b {
				t += a - a_buf
				a_buf = a
//				fmt.Printf("couple recv %d %f\n", recv_cnt, a);
				recv_cnt++
			} else {
				t += 3.0*float64(rand.ExpFloat64())
			}
//			fmt.Printf("couple send %f\n", t);
			select {
			case <- ain.end: break L1
			case <- out.end: break L1
			case out.vpath <- t:
			}
		}
		fmt.Printf("closed couple\n")
		close(in.end)
		close(out.end)
	}()
}
*/
func init_func(d float64, i int) (func(_, _ chain.Chan)(bool)) {
	t, j := 0.0, 0
	return func(_, in chain.Chan) (fin bool) {
		if j++; j >= i {
			in <- tupple{ -1, 0 }
			fmt.Printf("[END] generate\n")
			fin = true
			return
		}
		t += d*float64(rand.ExpFloat64())
		in <- tupple{ j, t }
		fin = false
		return
	}
}

func combine () (func(_, _, _ chain.Chan) (bool)) {
	i := 0
	t_buf := 0.0
	close_either := false
	var in_last, in_other chain.Chan = nil, nil
	return func(in, in2, out chain.Chan) (fin bool) {
		if in_last == nil { in_last, in_other = in, in2 }
		t := (<- in_last).(tupple)
		if t.Num < 0 {
			if close_either {
				out <- t
				fin = true
				return
			} else {
				close_either = true
				in_last, in_other = in_other, in_last
				return
			}
		}
		if !close_either && t.Time > t_buf {
			t.Time, t_buf = t_buf, t.Time
			in_last, in_other = in_other, in_last
		}
		t.Num = i
		out <- t
		i++
		fin = false
		return
	}
}

func end_func (cend chain.Chan) (fin bool) {
	t := (<- cend).(tupple)
	fin = t.Num < 0
	return
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


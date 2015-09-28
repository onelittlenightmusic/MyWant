package main
import (
	"fmt";
	"math/rand";
//	"runtime"
)
var max = 1000000.0
//type packet struct { time float64; end bool }
type stat struct { out_num int; interval_sum float64; interval_sqsum float64; queued_num int }
type queue struct { time_last_out float64; in chan float64; out chan float64; stat }
type path struct { vpath chan float64; end chan bool; }

func newpath() (path) {
	return path { make (chan float64), make (chan bool) }
}
func newasyncpath(n int) (path) {
	return path { make (chan float64, n), make (chan bool) }
}
func junction(in1, in2 path) (path) {
	out := newpath();
	in, in_buf := in1, in2;
	t, t_buf := 0.0, <-in.vpath;
	go func() {
		L1: for {
			select {
			case <- in.end: break L1;
			case <- in_buf.end: break L1;
			case <- out.end: break L1;
			case t = <- in.vpath:
			}
			if t > t_buf {
				t, t_buf = t_buf, t;
				in, in_buf = in_buf, in;
			}
			select {
			case <- in.end: break L1;
			case <- in_buf.end: break L1;
			case <- out.end: break L1;
			case out.vpath <- t:
			}
		}
		fmt.Printf("closed junction\n");
		close(in1.end);
		close(in2.end);
		close(out.end);
	}();
	return out;
}
func combine (in []path) (path) {
	switch x:= len(in); {
	case x==1: return in[0];
	case x>1:
		in_buf := in[0];
		for j := 1; j < x; j++ {
			in_buf = junction(in_buf, in[j]);
		}
		return in_buf;
	}
	return path{nil, nil};
}

func twice (in path) (path, path) {
	out1 := newpath();
	out2 := newpath();
	go func() {
		t := 0.0;
		L: for {
			select {
			case <- in.end: break L;
			case <- out1.end: break L;
			case <- out2.end: break L;
			case t = <- in.vpath:
			}
			select {
			case <- in.end: break L;
			case <- out1.end: break L;
			case <- out2.end: break L;
			case out1.vpath <- t:
			}
			select {
			case <- in.end: break L;
			case <- out1.end: break L;
			case <- out2.end: break L;
			case out2.vpath <- t:
			}
		}
		close(in.end);
		close(out1.end);
		close(out2.end);
	}();
	return out1, out2
}
func (qs *stat) log (interval float64) {
	qs.out_num++;
	qs.interval_sum += interval;
	qs.interval_sqsum += interval*interval
}
func (qs *stat) delay_count (in_base, in path) (path) {
	out := newpath();
	go func() {
		in_buf, in_buf2 := in_base, in;
		k, k_e:= -1, 1;
		t, t_buf:= 0.0, <- in.vpath;
		L: for {
			select {
			case <- in_buf.end: break L;
			case <- in_buf2.end: break L;
			case t = <- in_buf.vpath:
				k += k_e;
			}
			if t > t_buf {
				in_buf, in_buf2 =in_buf2, in_buf;
				t_buf = t;
				qs.queued_num += k;
				k_e = -k_e;
			}
		}
		close(in_base.end);
		close(in.end);
		close(out.end);
	}();
	return out;
}
func (q *queue) pass (in path) (path) {
	out := newpath();
	go func() {
		t_buf, t_ave := 0.0, 0.5;
		var t float64;
		L: for {
			select {
			case <- in.end: break L;
			case <- out.end: break L;
			case t = <- in.vpath:
			}
			if t > t_buf { t_buf = t }
			t_buf += t_ave*float64(rand.ExpFloat64());
			q.log(t_buf - t);
			select {
			case <- in.end: break L;
			case <- out.end: break L;
			case out.vpath <- t_buf:
			}
		}
		close(in.end);
		close(out.end);
	}();
	return out;
}
func filter(in path, p float64) (path) {
	out := newpath();
	_p := p;
	switch {
	case _p > 1: _p = 1
	case _p < 0: _p = 0
	}
	go func() {
		t := 0.0;
		L : for {
			select {
			case <- in.end: break L;
			case <- out.end: break L;
			case t = <- in.vpath:
			}
			if _p < rand.Float64() { continue L }
			select {
			case <- in.end: break L;
			case <- out.end: break L;
			case out.vpath <- t:
			}
		}
		fmt.Printf("closed filter\n");
		close(in.end);
		close(out.end);
	}();
	return out;
}
func async(in path, n int) (path) {
	out := newasyncpath(n);
	go func() {
		t := 0.0;
		L : for {
			select {
			case <- in.end: break L;
			case <- out.end: break L;
			case t = <- in.vpath:
			}
			select {
			case <- in.end: break L;
			case <- out.end: break L;
			case out.vpath <- t:
			}
		}
		close(in.end);
		close(out.end);
	}();
	return out;
}
func couple(in, out path) {
	ain := async(in,100);
	go func() {
		t, a_buf := 0.0, 0.0;
		recv_cnt:= 0;
		L1 : for {
			a, b := <- ain.vpath;
			if b {
				t += a - a_buf;
				a_buf = a;
//				fmt.Printf("couple recv %d %f\n", recv_cnt, a);
				recv_cnt++;
			} else {
				t += 3.0*float64(rand.ExpFloat64());
			}
//			fmt.Printf("couple send %f\n", t);
			select {
			case <- ain.end: break L1;
			case <- out.end: break L1;
			case out.vpath <- t:
			}
		}
		fmt.Printf("closed couple\n");
		close(in.end);
		close(out.end);
	}();
}
func generate(i int, d float64) (path) {
	out := newpath();
	go func() {
		t := 0.0;
		L : for j:=0; j<i; j++ {
			t += d*float64(rand.ExpFloat64());
			select {
			case <- out.end: break L;
			case out.vpath <- t:
//				fmt.Printf("generate send %d, %f\n", j, t);
			}
		}
		fmt.Printf("closed generate\n");
		close(out.end);
	}();
	return out;
}
func terminate(in path) (path){
	final := newpath();
	go func() {
		L: for {
			select {
			case <- in.end: break L;
			case <- in.vpath:
			}
		}
		close(final.end);
	}();
	return final;
}

func mkq(ins []path) (*queue, path) {
	rand.Seed(3);
	q := new(queue);
/*	in3 := newpath();
	ins2 := make([]path,len(ins)+1);
	for i:=0; i<len(ins); i++ {
		ins2[i] = ins[i];
	}
	ins2[len(ins2)-1] = in3;
*/
	mid1 := combine(ins);
	mid1_5, bufmid1 := twice ( mid1 );
	mid2 := q.pass(mid1_5);
//	mid2 := q.pass(mid1);
	mid3, bufmid2 := twice (mid2);
	last := q.delay_count (async(bufmid1, 100), async(bufmid2, 100));
	out := newpath();
	last.vpath = mid3.vpath;
	go func() {
		//waiting the last packet passing the queue.
		<-mid3.end;
		<-last.end;
		//sending TERMINATE signal to out path.
		close(out.end);
	}();
	return q, out;
}
func display_log (q *queue) () {
	ave:=q.interval_sum/float64(q.out_num);
	fmt.Printf("{\n");
	fmt.Printf("\tAve:\t%f\n", ave);
	fmt.Printf("\tVar:\t%f\n", q.interval_sqsum/float64(q.out_num)-ave*ave);
	fmt.Printf("\tQueued Num Ave:\t%f\n", float64(q.queued_num)/float64(q.out_num));
	fmt.Printf("\tNum:\t%d\n", q.out_num);
	fmt.Printf("}\n");
}
func main() {
	repeat := 1000;
	p1 := generate(repeat, 2.0);
	q1, p2 := mkq ([]path{ p1 });
//	p3 := newpath();
//	q2, p4 := mkq ([]path{ p2/*, p3 */});
//	p5, p6 := twice (p4);
//	couple (filter (p5, 0.3), p3);
//	p7 := terminate(p4);
	p3 := terminate(p2);
	<- p3.end;
	display_log(q1);
//	display_log(q2);
}




package main
import ( "fmt"; "rand"; "runtime" )
//type packet struct { time float; end bool }
type stat struct { out_num int; interval_sum float; interval_sqsum float; queued_num int }
type queue struct { time_last_out float; in chan float; out chan float; qs *stat }
type path struct { vpath chan int; end chan bool; }
func recv(a path) int {
	select {
	case n := <- a.vpath:
		return n;
	case <- a.end:
		fmt.Printf("kill recv\n");
		runtime.Goexit();
	}
	return 0;
}
func send(b path, n int) {
	select {
	case b.vpath <- n:
		return;
	case <- b.end:
		fmt.Printf("kill send\n");
		runtime.Goexit();
	}
	return;
}
func junction (in1, in2 <-chan float, i int) (chan float) {
	out := make(chan float, 1);
	in, in_buf := in1, in2;
	t, t_buf := 0.0, <- in2;
	go func() {
		for k := 0; k < i; k++ {
			t = <- in;
			if t > t_buf {
				t, t_buf = t_buf, t;
				in, in_buf = in_buf, in;
			}
			out <- t;
		}
	}();
	return out;
}
func combine2(in []<-chan float, i int) (<-chan float) {
	switch x:= len(in); {
	case x==1: return in[0];
	case x>1:
		in_buf := in[0];
		for j := 1; j < x; j++ {
			in_buf = junction(in_buf, in[j], i*(j+1));
		}
		return in_buf;
	}
	return nil;
}

func double (in <-chan float, i int) (chan float, chan float) {
	out1, out2 := make(chan float), make(chan float, 10);
	go func() {
		for j:=0; j<i; j++ {
			t := <- in;
			out1 <- t;
			out2 <- t;
//			for k := range out { out[k] <- t }
		}
	}();
	return out1, out2
}
func (qs *stat) log (interval float) {
	qs.out_num++;
	qs.interval_sum += interval;
	qs.interval_sqsum += interval*interval
}
func (qs *stat) delay_count2 (end chan<- bool, in <-chan float, i int) {
	return;
}
func (qs *stat) delay_count (in_base, in <-chan float, i int) (chan bool) {
	end := make(chan bool, 1);
	go func() {
		in_buf, in_buf2 := in, in_base;
		k := -1;
		for j := 0; j < i; j++ {
/*			k++;
			base := <- in_base;
			L: for {
				if base < buf {
					qs.queued_num += k;
					break L;
				}
				k--;
				buf = <- in;
			}
			qs.out_num++;
*/
			in_buf, in_buf2 =in_buf2, in_buf;
			<-in_buf;
			qs.queued_num += k;
		}
		end<- true;
	}();
	return end;
}
func (q *queue) pass (middle <-chan float, i int) (chan float) {
	out := make(chan float);
	go func() {
		t_buf, t_ave := 0.0, 0.5;
		for j := 0; j < i; j++ {
			t := <- middle;
			if t > t_buf { t_buf = t }
			t_buf += t_ave*float(rand.ExpFloat64());
			out <- t_buf;
			q.qs.log(t_buf - t);
		}
	}();
	return out;
}
func generate(i int) (chan float) {
	out := make(chan float);
	go func() {
		t, t_ave := 0.0, 2.0;
		for j := 0; j < i; j++ { out <- t; t += t_ave*float(rand.ExpFloat64()) }
		out <- t_ave*float(i)*20.0;
	}();
	return out;
}

func main() {
	q, q2 := new(queue), new(queue);
	q.qs, q2.qs = new(stat), new(stat);
//	q.in, q.out = make(chan float), make(chan float);
	repeat := 1000;
	rand.Seed(2);
	in1 := generate(repeat);
	in2 := generate(repeat);
	in3 := generate(repeat);
	middle3 := combine2([]<-chan float{ in1, in2, in3 }, repeat);
	middle2, out2 := double( middle3, 3*repeat);
	out := q.pass(middle2, 3*repeat);
	out3, out4 := double(out, 3*repeat);
	end := q2.qs.delay_count(out2, out4, 6*repeat);
	go func() {
		for j := 0; j < 3*repeat; j++ { <-out3;/* <-out; fmt.Printf("%f\n", <- out)*/ }
	}();
	<-end;
	ave:=q.qs.interval_sum/float(q.qs.out_num);
	fmt.Printf("Ave %f\nVar %f\n", ave, q.qs.interval_sqsum/float(q.qs.out_num)-ave*ave);
	fmt.Printf("Queued Num Ave %d\n", float(q2.qs.queued_num)/float(q2.qs.out_num));
}


package main
import ( "fmt"; "rand"; "runtime" )
type packet struct { time float; end bool }
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
func junction (out chan<- float, in1, in2 <-chan float, i int) {
	in, in_buf := in1, in2;
	t, t_buf := 0.0, <- in2;
	for k := 0; k < i; k++ {
		t = <- in;
		if t > t_buf {
			t, t_buf = t_buf, t;
			in, in_buf = in_buf, in;
		}
		out <- t;
	}
}
func combine2(in []<-chan float, i int) (<-chan float) {
	buf := make(chan float);
	if len(in)<=0 { return nil }
	else if len(in)==1 { return (in[0]) }
	for j := 0; j < len(in)-2; j++ {
		middle := make(chan float,1);
		go junction(buf, middle, in[j], i*(len(in)-j));
		buf = middle;
	}
	go junction(buf, in[len(in)-2], in[len(in)-1], i*2);
	return buf;
}

func double (out []chan<- float, in <-chan float, i int) {
	go func() {
		for j:=0; j<i; j++ {
			t := <- in;
			for k := range out { out[k] <- t }
		}
	}();
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
	end := make(chan bool);
	go func() {
		buf := <- in;
		k := -1;
		for j := 0; j < i; j++ {
			k++;
			base := <- in_base;
			L: for ; ; {
				if base < buf {
					qs.queued_num += k;
					break L;
				}
				k--;
				buf = <- in;
			}
			qs.out_num++;
		}
		end<- true;
	}();
	return end;
}
func (q *queue) pass (out chan<- float, middle <-chan float, i int) {
	t_buf, t_ave := 0.0, 0.5;
	for j := 0; j < i; j++ {
		t := <- middle;
		if t > t_buf { t_buf = t }
		t_buf += t_ave*float(rand.ExpFloat64());
		out <- t_buf;
		q.qs.log(t_buf - t);
	}
}
func generate(i int) (chan float) {
	out := make(chan float);
	go func() {
		t, t_ave := 0.0, 2.0;
		for j := 0; j < i; j++ { out <- t; t += t_ave*float(rand.ExpFloat64()) }
		out <- t_ave*float(i)*5.0;
	}();
	return out;
}

func main() {
	q, q2 := new(queue), new(queue);
	q.qs, q2.qs = new(stat), new(stat);
//	q.in, q.out = make(chan float), make(chan float);
	repeat := 10000;
	rand.Seed(2);
	out:= make(chan float);
	middle2 := make(chan float);
//	middle3 := make(chan float);
	out2:= make(chan float,100);
	out3, out4 := make(chan float,100), make(chan float);
	in1 := generate(repeat);
	in2 := generate(repeat);
	in3 := generate(repeat);
	middle3 := combine2([]<-chan float{ in1, in2, in3 }, repeat);
	double([]chan<- float { middle2, out2 }, middle3, 3*repeat);
	go q.pass(out, middle2, 3*repeat);
	double([]chan<- float {out3, out4}, out, 3*repeat);
	end := q2.qs.delay_count(out2, out3, 3*repeat);
	for j := 0; j < 3*repeat; j++ { <-out4;/* <-out; fmt.Printf("%f\n", <- out)*/ }
	<-end;
	ave:=q.qs.interval_sum/float(q.qs.out_num);
	fmt.Printf("Ave %f\nVar %f\n", ave, q.qs.interval_sqsum/float(q.qs.out_num)-ave*ave);
	fmt.Printf("Queued Num Ave %d\n", float(q2.qs.queued_num)/float(q2.qs.out_num));
}


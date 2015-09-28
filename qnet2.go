package main
import ( "fmt"; "rand" )
type packet struct { time float; end bool }
type stat struct { out_num int; interval_sum float; interval_sqsum float; queued_num int }
type queue struct { time_last_out float; in chan float; out chan float; qs *stat }
func junction (out chan<- packet, in1, in2 <-chan packet, i int) {
	in, in_buf := in1, in2;
	t, t_buf := packet{ 0.0, false }, <- in2;
	for k := 0; k < i; k++ {
		t = <- in;
		if t.time > t_buf.time {
			t, t_buf = t_buf, t;
			in, in_buf = in_buf, in;
		}
		out <- t;
	}
}
func combine2(out chan<- packet, in []<-chan packet, i int) {
	buf := out;
	if len(in)<2 { return }
	for j := 0; j < len(in)-2; j++ {
		middle := make(chan packet,1);
		go junction(buf, middle, in[j], i*(len(in)-j));
		buf = middle;
	}
	go junction(buf, in[len(in)-2], in[len(in)-1], i*2);
}

func double (out []chan<- packet, in <-chan packet, i int) {
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
func (qs *stat) delay_count (end chan<- bool, in_base, in <-chan packet, i int) {
	buf := <- in;
	k := -1;
	for j := 0; j < i; j++ {
		k++;
		base := <- in_base;
		L: for ; ; {
			if base.time < buf.time {
				qs.queued_num += k;
				break L;
			}
			k--;
			buf = <- in;
		}
		qs.out_num++;
	}
	end<- true;
}
func (q *queue) pass (out chan<- packet, middle <-chan packet, i int) {
	t_buf, t_ave := packet{ 0.0, false }, 0.5;
	for j := 0; j < i; j++ {
		t := <- middle;
		if t.time > t_buf.time { t_buf = t }
		t_buf.time += t_ave*float(rand.ExpFloat64());
		out <- t_buf;
		q.qs.log(t_buf.time - t.time);
	}
}
func generate(i int) (out chan packet) {
	out := make(chan packet);
	go func() {
		t, t_ave := packet{ 0.0, false }, 2.0;
		for j := 0; j < i; j++ { ch <- t; t.time += t_ave*float(rand.ExpFloat64()) }
		ch <- packet{ t_ave*float(i)*5.0,true };
	}();
}

func main() {
	q, q2 := new(queue), new(queue);
	q.qs, q2.qs = new(stat), new(stat);
//	q.in, q.out = make(chan packet), make(chan packet);
	repeat := 10000;
	rand.Seed(2);
//	in1, in2, in3, out:= make(chan packet), make(chan packet), make(chan packet), make(chan packet);
	out:= make(chan packet);
//	middle1 := make(chan float);
	middle2 := make(chan packet);
	middle3 := make(chan packet);
	out2:= make(chan packet,100);
	out3, out4 := make(chan packet,100), make(chan packet);
	end := make(chan bool);
	in1 := generate(repeat);
	in2 := generate(in2, repeat);
	in3 := generate(in3, repeat);
	combine2(middle3, []<-chan packet{ in1, in2, in3 }, repeat);
	double([]chan<- packet { middle2, out2 }, middle3, 3*repeat);
	go q.pass(out, middle2, 3*repeat);
	double([]chan<- packet {out3, out4}, out, 3*repeat);
	go q2.qs.delay_count(end, out2, out3, 3*repeat);
	for j := 0; j < 3*repeat; j++ { <-out4;/* <-out; fmt.Printf("%f\n", <- out)*/ }
	<-end;
	ave:=q.qs.interval_sum/float(q.qs.out_num);
	fmt.Printf("Ave %f\nVar %f\n", ave, q.qs.interval_sqsum/float(q.qs.out_num)-ave*ave);
	fmt.Printf("Queued Num Ave %d\n", float(q2.qs.queued_num)/float(q2.qs.out_num));
}

func combine (out chan<- packet, in []<-chan packet, i int) {
	middle := make(chan packet, 10);
	if len(in)>2 {
		go junction(out, in[len(in)-1], middle, i*len(in));
		combine(middle, in[0:len(in)-2], i);
	} else {
		go junction(out, in[0], in[1], i*2);
	}
}

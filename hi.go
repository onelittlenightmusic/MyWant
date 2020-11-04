package main
import (
//	"exec";
//	"io/ioutil";
	"fmt";
//	"os";
)

type tnode struct { time float64 }
type qstat struct { out_num int; interval_ave float64; interval_var float64 }

func main() {
//1	fmt.Printf("こんにちは、世界!!\n")
	ch := make(chan int);
	var q qstat;
	go func(i int) {
		/*for j := 0; j < i; j++ */{
//			j := 0;
			a := <- ch;
			fmt.Printf("# -> %d\n", a);
		}
	}(100);
	go func(i int) {
		for j := 0; j < i; j++ { q.out_num += j; /*ch <- q.out_num*/ };
//2		fmt.Printf("Total %d\n",q.i);
		ch <- q.out_num;
	}(100); // Passes argument 1000 to the function literal.
	fmt.Printf("Total %d\n",q.out_num);
//	os.Exec("dir", nil, nil, nil, nil);
//	println("end");
/*	if cmd, e := exec.Run("/bin/ls", nil, nil, exec.DevNull, exec.Pipe, exec.MergeWithStdout); e == nil {
		b, _ := ioutil.ReadAll(cmd.Stdout);
		println("output: " + string(b));
	}
*/
}

package main
import (
	"fmt";
	"rand";
	"math";
)
var num int = 1000
var theta float64 = 0.1
type packet struct {
	x float64;
	end bool;
}

func main() {
	var (
		sum, variance, alpha float64 = 0.0, 0.0, 0.05;
		path = make(chan packet, 100);
		endall = make(chan bool);
	);
	go func(k int) {
		for i := 0; i<k; i++ { path <- packet{math.Sin(theta * float64(i)) + alpha*rand.NormFloat64(),false}; }
	//		fmt.Printf("%f\n", a);
		path <- packet{0.0, true};
	}(num);
	go func(k int) {
		for {
			a := <- path;
			if a.end { endall <- true; break }
			sum += a.x;
			variance += a.x*a.x;
		}
	}(num);
	<- endall;
	fmt.Printf("mean value : %f\n", sum/float64(num));
	fmt.Printf("variance : %f\n", variance/float64(num));
}
		
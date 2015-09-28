package main
import (
	"fmt"
)

type session struct{
	c chan interface{}
}

func test_print(a interface{}) {
	fmt.Printf("test print ok")
	return
}

func test_print2(a chan interface{}) {
	fmt.Printf("test print 2 ok")
	return 
}

func test_print3(a session) {
	fmt.Printf("test print 2 ok")
	return 
}

func main() {
	test_print("test");
//	test_print2(make(chan int));
	a := session{ make(chan interface{},1) }
	test_print(a)
	a.c <- 1
	<- a.c
	return
}

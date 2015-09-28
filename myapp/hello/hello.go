package hello

import (
	"fmt"
	"http"
)

func init() {
	http.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) () {
		fmt.Fprint(w, "Hello, world!")
	})
}


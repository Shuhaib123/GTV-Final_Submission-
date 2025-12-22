//go:build ignore

package main

import (
	"fmt"
	"go/format"
	"jspt/internal/instrumenter"
)

func main() {
	src := `package main
import "context"
func main() {
	ctx := context.Background()
	ch := make(chan int)
	ch <- 1
	_ = <-ch
}`
	instrumenter.SetOptions(instrumenter.Options{Level: "regions_logs"})
	out, err := instrumenter.InstrumentProgram([]byte(src), "Demo")
	if err != nil {
		panic(err)
	}
	fmt.Println("raw:\n" + string(out))
	fmt.Println("---- formatted ----")
	formatted, fmtErr := format.Source(out)
	if fmtErr != nil {
		panic(fmtErr)
	}
	fmt.Println(string(formatted))
}

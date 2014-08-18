package main

import (
	"fmt"
	"time"
)

var debug bool = false

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s took %s", name, elapsed)
}

func init() {
	debug = cmdlineBool("debug")
}

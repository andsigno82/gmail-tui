package main

import (
	"fmt"
	"os"
)

// version viene iniettata a build time: -ldflags "-X main.version=0.1.0"
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println("gmail-tui " + version)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "gmail-tui is under construction; available: version")
	os.Exit(1)
}

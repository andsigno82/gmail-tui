package main

import (
	"fmt"
	"os"

	"gmail-tui/internal/cmd"
	"gmail-tui/internal/config"
)

// version viene iniettata a build time: -ldflags "-X main.version=0.1.0"
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println("gmail-tui " + version)
			return
		case "themes":
			cmd.Themes(os.Stdout)
			return
		case "doctor":
			cfgPath, err := config.Path()
			if err != nil {
				fmt.Fprintf(os.Stderr, "config path: %v\n", err)
				os.Exit(1)
			}
			os.Exit(cmd.Doctor(os.Stdout, cfgPath, os.Getenv))
		}
	}
	fmt.Fprintln(os.Stderr, "comandi: version | themes | doctor")
	os.Exit(1)
}

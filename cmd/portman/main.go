package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "daemon":
		runDaemon(os.Args[2:])
	case "add", "rm", "ls":
		runCli()
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
	portman daemon [-config path]    start the daemon
	postmand add <user@host> <local:remote>
	postmand rm <local>
	postmand ls
`)
}

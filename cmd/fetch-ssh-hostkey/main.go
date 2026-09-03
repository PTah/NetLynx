package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprintln(os.Stderr, "usage: fetch-ssh-hostkey <host> [port]")
		fmt.Fprintln(os.Stderr, "tries modern, then transitional, then legacy SSH algorithms")
		os.Exit(2)
	}
	host := os.Args[1]
	port := 22
	if len(os.Args) >= 3 {
		n, err := strconv.Atoi(os.Args[2])
		if err != nil || n <= 0 || n > 65535 {
			fmt.Fprintln(os.Stderr, "invalid port")
			os.Exit(2)
		}
		port = n
	}
	line, profile, err := swcfg.FetchHostKeyLineWithProfile(host, port, 12*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if os.Getenv("FETCH_SSH_HOSTKEY_VERBOSE") == "1" {
		fmt.Fprintf(os.Stderr, "profile=%s\n", profile)
	}
	fmt.Println(line)
}

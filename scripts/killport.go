//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	port := "8080"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	// Run netstat to find PID
	out, err := exec.Command("netstat", "-aon").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "netstat failed: %v\n", err)
		os.Exit(1)
	}

	var pid int
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ":"+port) && strings.Contains(line, "LISTENING") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				pid, _ = strconv.Atoi(fields[len(fields)-1])
				break
			}
		}
	}

	if pid == 0 {
		fmt.Printf("No process listening on port %s\n", port)
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FindProcess(%d): %v\n", pid, err)
		os.Exit(1)
	}

	fmt.Printf("Killing PID %d on port %s...\n", pid, port)
	if err := proc.Kill(); err != nil {
		fmt.Fprintf(os.Stderr, "Kill failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done")
}

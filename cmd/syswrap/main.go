package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/netapp/trident/internal/syswrap/unix"
)

var syscalls = map[string]func([]string) (interface{}, error){
	"statfs": unix.Statfs,
	"exists": unix.Exists,
}

type result struct {
	output interface{}
	err    error
}

func main() {
	timeout, syscall, args, err := parseArgs(os.Args)
	if err != nil {
		exit(err)
	}

	select {
	case <-time.After(timeout):
		exit(fmt.Errorf("timed out waiting for %s", syscall))
	case res := <-func() chan result {
		r := make(chan result)
		go func() {
			s, ok := syscalls[syscall]
			if !ok {
				r <- result{
					err: fmt.Errorf("unknown syscall: %s", syscall),
				}
			}
			i, e := s(args)
			r <- result{
				output: i,
				err:    e,
			}
		}()
		return r
	}():
		if res.err != nil {
			exit(res.err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(res.output)
	}
}

func exit(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "error: %v", err)
	os.Exit(1)
}

func parseArgs(args []string) (timeout time.Duration, syscall string, syscallArgs []string, err error) {
	if len(args) < 3 {
		err = fmt.Errorf("expected at least 3 arguments")
		return
	}
	timeout, err = time.ParseDuration(args[1])
	if err != nil {
		return
	}

	syscall = args[2]

	if len(args) > 3 {
		syscallArgs = args[3:]
	}
	return
}

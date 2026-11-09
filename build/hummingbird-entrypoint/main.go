// The hardened image has no shell. This launcher preserves entrypoint.sh's
// argument and APP_ENV behavior and replaces itself so signals reach the API.
package main

import (
	"fmt"
	"os"
	"syscall"
)

func arguments(env string, extra []string) []string {
	args := []string{"./rhobs-synthetics-api", "start"}
	if env == "dev" {
		args = append(args, "--database-engine", "local")
	}
	return append(args, extra...)
}

func main() {
	args := arguments(os.Getenv("APP_ENV"), os.Args[1:])
	if err := syscall.Exec(args[0], args, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

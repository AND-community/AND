//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// baslat, AND'ı masaüstü arayüzüyle başlatır ve kapanmasını bekler.
func baslat(and string) error {
	cmd := exec.Command(and, "--masaustu")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func hataGoster(mesaj string) { fmt.Fprintln(os.Stderr, "and-masaustu:", mesaj) }

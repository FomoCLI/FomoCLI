package ui

import (
	"os"

	"golang.org/x/term"
)

func termSize() (int, int, error) { return term.GetSize(int(os.Stdout.Fd())) }

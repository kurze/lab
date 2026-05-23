package main

import (
	"fmt"
	"os"
	"time"
)

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
)

var colorEnabled = isTerminal()

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func cl(code, text string) string {
	if !colorEnabled {
		return text
	}
	return code + text + ansiReset
}

func logf(format string, args ...any) {
	ts := cl(ansiDim, time.Now().Format("2006/01/02 15:04:05"))
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s %s\n", ts, msg)
}

func warnf(format string, args ...any) {
	ts := cl(ansiDim, time.Now().Format("2006/01/02 15:04:05"))
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s %s %s\n", ts, cl(ansiYellow, "warning:"), msg)
}

func errf(format string, args ...any) {
	ts := cl(ansiDim, time.Now().Format("2006/01/02 15:04:05"))
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s %s %s\n", ts, cl(ansiRed, "error:"), msg)
}

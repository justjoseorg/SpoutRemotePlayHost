//go:build !windows

package main

import "log"

func setupLog(string) {}

func fatal(err error) { log.Fatal(err) }

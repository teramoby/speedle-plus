package main

import "testing"

func TestPrintVersionInfo(t *testing.T) {
	productVersion, goVersion, gitCommit = "1.0", "go1.26", "abc"
	printVersionInfo()
}

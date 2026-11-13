// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build !unix

package adsync

func kernelVersion(func(string) ([]byte, error)) string { return "" }

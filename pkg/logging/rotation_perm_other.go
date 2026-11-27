//go:build !unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

func (w *rotationWriter) initPlatform() {}

func (w *rotationWriter) writePlatform(p []byte) (int, error) {
	return w.logger.Write(p)
}

func (w *rotationWriter) rotatePlatform() error {
	return w.logger.Rotate()
}

func (w *rotationWriter) closePlatform() error {
	return w.logger.Close()
}

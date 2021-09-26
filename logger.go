/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import "fmt"

// logger is a logger interface compatible with both stdlib and some 3rd party loggers.
type logger interface {
	Output(calldepth int, s string) error
}

// ilogger represents the internal logging api we use.
type ilogger interface {
	logger
	Print(...interface{})
}

// internalLog implements the additional methods used by our internal logging.
type internalLog struct {
	logger
}

// Print replicates the behaviour of the standard logger.
func (t internalLog) Print(v ...interface{}) {
	t.Output(2, fmt.Sprint(v...))
}

type discard struct{}

func (t discard) Output(int, string) error { return nil }
func (t discard) Print(...interface{})     {}

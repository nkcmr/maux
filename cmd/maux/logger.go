package main

import (
	"fmt"
	"log"
	"os"
)

type logLevel string

func (l logLevel) String() string {
	return string(l)
}

const (
	ldebug logLevel = "debug"
	linfo           = "info"
	lwarn           = "warn"
	lerror          = "error"
	lfatal          = "fatal"
)

type logger interface {
	WithPrefix(prefix string) logger
	Write(level logLevel, format string, a ...interface{})
}

type defLogger struct {
	prefix string
	ll     *log.Logger
}

func newDefLogger() *defLogger {
	return &defLogger{
		prefix: "root",
		ll:     log.New(os.Stderr, "", 0),
	}
}

func (d defLogger) WithPrefix(prefix string) logger {
	d.prefix = prefix
	return &d
}

func (lg defLogger) Write(level logLevel, format string, a ...interface{}) {
	lg.ll.Printf(fmt.Sprintf("%s: (%s) %s", level.String(), lg.prefix, format), a...)
}

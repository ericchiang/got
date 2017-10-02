package log

import (
	"log"
	"os"
)

type Level int

const (
	Silent = iota
	Error
	Info
	Debug
)

// Logger represents a logging strategy. This should be used to indicate
// a struct or method can log options instead of
type Logger interface {
	Infof(format string, v ...interface{})
	Debugf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
}

func New(level int) Logger {
	const flags = log.LstdFlags
	l := &logger{}
	if level >= Error {
		l.error = log.New(os.Stderr, "[error] ", flags)
	}
	if level >= Info {
		l.info = log.New(os.Stderr, "[info] ", flags)
	}
	if level >= Debug {
		l.debug = log.New(os.Stderr, "[debug] ", flags)
	}
	return l
}

type logger struct {
	info  *log.Logger
	debug *log.Logger
	error *log.Logger
}

func (l *logger) Infof(format string, v ...interface{})  { print(l.info, format, v...) }
func (l *logger) Debugf(format string, v ...interface{}) { print(l.debug, format, v...) }
func (l *logger) Errorf(format string, v ...interface{}) { print(l.error, format, v...) }

func print(l *log.Logger, format string, v ...interface{}) {
	if l != nil {
		l.Printf(format, v...)
	}
}

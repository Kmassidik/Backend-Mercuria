package logger

import (
	"log"
	"os"
)


type Logger struct {
	info *log.Logger
	warn *log.Logger
	error *log.Logger
	debug *log.Logger
}

func New(serviceName string) *Logger {
	prefix := "[" + serviceName + "] "

	return &Logger{
		info: log.New(os.Stdout, prefix+"INFO: ", log.Ldate|log.Ltime|log.Lshortfile),
		warn: log.New(os.Stdout, prefix+"WARN: ", log.Ldate|log.Ltime|log.Lshortfile),
		error: log.New(os.Stdout, prefix+"ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
		debug: log.New(os.Stdout, prefix+"DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile),
	}
}

func (l *Logger) Info(v ...interface{}) {
	l.info.Println(v...)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.info.Printf(format, v...)
}

func (l *Logger) Warn(v ...interface{}) {
	l.warn.Println(v...)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.warn.Printf(format, v...)
}

func (l *Logger) Error(v ...interface{}) {
	l.error.Println(v...)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.error.Printf(format, v...)
}

func (l *Logger) Debug(v ...interface{}) {
	l.debug.Println(v...)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.debug.Printf(format, v...)
}

// Fatal logs and exits
func (l *Logger) Fatal(v ...interface{}) {
	l.error.Fatal(v...)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.error.Fatalf(format, v...)
}



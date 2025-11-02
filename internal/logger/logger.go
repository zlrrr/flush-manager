package logger

import (
	"fmt"
	"log"
	"os"
)

const prefix = "[flush-manager]"

var (
	infoLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
)

func init() {
	infoLogger = log.New(os.Stdout, prefix+" INFO: ", log.Ldate|log.Ltime)
	errorLogger = log.New(os.Stderr, prefix+" ERROR: ", log.Ldate|log.Ltime)
	debugLogger = log.New(os.Stdout, prefix+" DEBUG: ", log.Ldate|log.Ltime)
}

// Info logs an info message
func Info(format string, v ...interface{}) {
	infoLogger.Printf(format, v...)
}

// Error logs an error message
func Error(format string, v ...interface{}) {
	errorLogger.Printf(format, v...)
}

// Debug logs a debug message
func Debug(format string, v ...interface{}) {
	debugLogger.Printf(format, v...)
}

// Infof logs an info message (alias for compatibility)
func Infof(format string, v ...interface{}) {
	Info(format, v...)
}

// Errorf logs an error message (alias for compatibility)
func Errorf(format string, v ...interface{}) {
	Error(format, v...)
}

// Debugf logs a debug message (alias for compatibility)
func Debugf(format string, v ...interface{}) {
	Debug(format, v...)
}

// Fatal logs an error message and exits
func Fatal(format string, v ...interface{}) {
	errorLogger.Printf(format, v...)
	os.Exit(1)
}

// Printf logs to stdout with prefix
func Printf(format string, v ...interface{}) {
	fmt.Printf(prefix+" "+format, v...)
}

// Println logs to stdout with prefix
func Println(v ...interface{}) {
	fmt.Print(prefix + " ")
	fmt.Println(v...)
}

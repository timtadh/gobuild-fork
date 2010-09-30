// Copyright 2009-2010 by Maurice Gilden. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
 A collection of helper functions for console output.
*/
package logger

import "fmt"
import "os"

const (
	DEBUG   = -1 + iota // start with -1
	DEFAULT // should be 0
	WARN
	ERROR
)

var verbosity int

/*
 Sets the verbosity level. Possible values are DEBUG, DEFAULT, WARN and ERROR.
 DEFAULT is the standard value which will print everything but debug messages.
*/
func SetVerbosityLevel(level int) { verbosity = level }

/*
 Prints debug messages. Same syntax as fmt.Printf.
*/
func Debug(format string, v ...interface{}) {
	if verbosity <= DEBUG {
		fmt.Printf("DEBUG: ")
		fmt.Printf(format, v...)
	}
}

/*
 Prints a debug message if enabled but without the "DEBUG: " prefix.
 Same syntax as fmt.Printf.
*/
func DebugContinue(format string, v ...interface{}) {
	if verbosity <= DEBUG {
		fmt.Printf("       ")
		fmt.Printf(format, v...)
	}
}

/*
 Prints an info message if enabled. This is for general feedback about what
 gobuild is currently doing. Same syntax as fmt.Printf.
*/
func Info(format string, v ...interface{}) {
	if verbosity <= DEFAULT {
		fmt.Printf(format, v...)
	}
}


/*
 Prints a warning if warnings are enabled. Same syntax as fmt.Printf.
*/
func Warn(format string, v ...interface{}) {
	if verbosity <= WARN {
		fmt.Print("WARNING: ")
		fmt.Printf(format, v...)
	}
}

/*
 Prints a warning message if enabled but without the "WARNING: " prefix.
 Same syntax as fmt.Printf.
*/
func WarnContinue(format string, v ...interface{}) {
	if verbosity <= WARN {
		fmt.Print("         ")
		fmt.Printf(format, v...)
	}
}

/*
 Prints an error message. Same syntax as fmt.Printf.
*/
func Error(format string, v ...interface{}) {
	if verbosity <= ERROR {
		fmt.Fprint(os.Stderr, "ERROR: ")
		fmt.Fprintf(os.Stderr, format, v...)
	}
}

/*
 Prints an error message but without the "ERROR: " prefix.
 Same syntax as fmt.Printf.
*/
func ErrorContinue(format string, v ...interface{}) {
	if verbosity <= ERROR {
		fmt.Fprint(os.Stderr, "       ")
		fmt.Fprintf(os.Stderr, format, v...)
	}
}

package main_test

import (
	"fmt"
	"testing"
)

type TestFile struct {
	path string
	content string
}

func TestGobuild(t *testing.T) {
	// TODO: do something testy here
	fmt.Println("Here be some testing someday. Please?")
}

// test for a simple program with one file
func TestSimpleProgram(t *testing.T) {
	var prog = []TestFile{TestFile{"main.go", "package main\n\nfunc main() {\n\n}\n"}}
    writeTestFiles(prog)
}

// test for a simple library with one package
func TestSimpleLibrary(t *testing.T) {

}

// compiler error in application
func TestProgramCompilerError(t *testing.T) {

}

// linker error in application
func TestProgramLinkError(t *testing.T) {

}

// compiler error in library
func TestLibraryCompilerError(t *testing.T) {

}

// pack error in library
func TestLibraryPackError(t *testing.T) {

}

// test for the different import styles (with ./ or without)
func TestDifferentImports(t *testing.T) {

}

// test -I command line parameter with an application
func TestProgramWithInclude(t *testing.T) {

}

// test -I command line parameter with a library
func TestLibraryWithInclude(t *testing.T) {

}

// test with a more advanced program with many files & packages
func TestComplexApplication(t *testing.T) {

}

// test with a more complex library with multiple files & packages
func TestComplexLibrary(t *testing.T) {

}


func writeTestFiles(files []TestFile) {

}

func runGobuild() bool {

	return true
}

func BenchmarkGobuild(b *testing.B) {
        fmt.Println("Benchmarks would be nice too...")
}

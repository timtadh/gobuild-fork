// Copyright 2009 by Maurice Gilden. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
 gobuild - build tool to automate building go programs
*/
package main

import (
	os "os";
	"exec";
	"flag";
	"fmt";
	"path";
	"strings";
	"container/vector";
	"go/ast";
	"go/parser";
)

// ========== command line parameters ==========

var buildAllTargets *bool = flag.Bool("a", false, "(not yet implemented) build all targets");
var buildTesting *bool = flag.Bool("t", false, "(not yet implemented) Build all tests");
var includeAllMain *bool = flag.Bool("multiple-main", false, "use all main package files without main function");
var includeInvisible *bool = flag.Bool("include-hidden", false, "Include hidden directories");
var outputFileName *string = flag.String("o", "", "output file");
var quietMode *bool = flag.Bool("q", false, "only print warnings/errors");
var quieterMode *bool = flag.Bool("qq", false, "only print errors");
var verboseMode *bool = flag.Bool("v", false, "print debug messages");

// ========== constants ==========


// ========== structs ==========

type goPackage struct {
	name string;             // name of the package
	files *vector.Vector;    // a list of files for this package
	depends *vector.Vector;  // a list of other local packages this one depends on
	compiled bool;           // true = finished compiling
	inProgress bool;         // true = currently trying to compile dependencies (needed to find recursive dependencies)
	hasErrors bool;          // true = compiler returned an error
}

type goFile struct {
	filename string;         // file name with full path
	content []byte;          // file content
	pack *goPackage;         // the package this file belongs to
	hasMain bool;
}

// ========== global (package) variables ==========

var goFileVector vector.Vector;
var goPackageMap map[string] *goPackage;
var compilerBin string;
var linkerBin string;
var mainGoFileName string; // can also be a command line parameter
var compileError bool = false;
var linkError bool = false;
var rootPath string;

// ========== goFile methods ==========

/*
 Parses the content of a .go file and searches for package name, imports and main function.
*/
func (this *goFile) parseFile() (err os.Error) {
	var fileast *ast.File;
	var exists bool;
	var packageName string;

	fileast, err = parser.ParseFile(this.filename, nil, 0);

	if err != nil {
		warn("Parsing file %s returned with errors: %s\n", this.filename, err);
	}

	// exclude the file if "main" package and non-include-all
	if fileast.Name.String() == "main" &&
		!*includeAllMain && this.filename != mainGoFileName {
		return;
	}

	// check if package is in the correct path
	packageName = fileast.Name.String();
	if fileast.Name.String() != "main" {
		switch strings.Count(this.filename, "/") {
		case 0: // no sub-directory
			warn("File %s from package %s is not in the correct path. Should be %s.\n",
				this.filename, fileast.Name, 
				strings.Join([]string{fileast.Name.String(), this.filename}, "/"));
		case 1: // one sub-directory
			if this.filename[0:strings.Index(this.filename, "/")] != fileast.Name.String() {
				warn("File %s from package %s is not in the correct directory. Should be %s.\n",
					this.filename, fileast.Name.String(),
					strings.Join([]string{fileast.Name.String(),
					this.filename[strings.Index(this.filename, "/")+1:len(this.filename)]}, "/"));
			}
		default: // >1 sub-directories
			if this.filename[max(strings.LastIndex(this.filename, "/") - len(fileast.Name.String()), 0):strings.LastIndex(this.filename, "/")] != fileast.Name.String() {

				// NOTE: this case will result in a link-error (exit with error here?)
				warn("File %s from package %s is not in the expected directory.\n",
					this.filename, fileast.Name.String());
			}
			packageName = strings.Join([]string{
				this.filename[0:strings.LastIndex(this.filename[0:strings.LastIndex(this.filename, "/")], "/")],
				fileast.Name.String()}, "/");
		}
	}	

	// check if package is already known
	this.pack, exists = goPackageMap[packageName];
	if !exists {
		// create it if it's unknown
		this.pack = newGoPackage(packageName);
	}
	this.pack.files.Push(this);

	// find the local imports in this file
	visitor := astVisitor{this};
	ast.Walk(visitor, fileast);
	
	return;
}

// ========== goPackage methods ==========

/*
 Creates a new goPackage and adds it to goPackageMap.
 Doesn't check if there's already a package with that name.
*/
func newGoPackage(name string) *goPackage {
	pack := new(goPackage);
	pack.compiled = false;
	pack.inProgress = false;
	pack.hasErrors = false;
	pack.name = name;
	pack.files = new(vector.Vector);
	pack.depends = new(vector.Vector);
	goPackageMap[pack.name] = pack;
	return pack;
}

// ========== goFileVisitor ==========

// this visitor looks for files with the extension .go
type goFileVisitor struct {}

	
// implementation of the Visitor interface for the file walker
func (v *goFileVisitor) VisitDir(path string, d *os.Dir) bool {
	if path[strings.LastIndex(path, "/") + 1] == '.' {
		return *includeInvisible;
	}
	return true;
}

func (v *goFileVisitor) VisitFile(path string, d *os.Dir) {
	// parse hidden directories?
	if (path[strings.LastIndex(path, "/") + 1] == '.') && (!*includeInvisible) {
		return;
	}

	if strings.HasSuffix(path, ".go") {
		// include _test.go files?
		if strings.HasSuffix(path, "_test.go") && (!*buildTesting) {
			return;
		}

		gf := goFile{path[len(rootPath)+1:len(path)], nil, nil, false};
		gf.parseFile();
		goFileVector.Push(&gf);
	}
}

// ========== astVisitor ==========

// this visitor looks for imports and the main function in an AST
type astVisitor struct {
	file *goFile;
}

/*
 Implementation of the visitor interface for ast walker.
 Returning nil stops the walker, anything else continues into the subtree
*/
func (v astVisitor) Visit(node interface{}) (w ast.Visitor) {
	switch n := node.(type) {
	case *ast.ImportSpec:
		for _, bl := range n.Path {
			if (len(bl.Value) > 4) && 
				(bl.Value[1] == '.') &&
				(bl.Value[2] == '/'){
				
				// local package found
				packName := string(bl.Value[3:len(bl.Value)-1]);
				dep, exists := goPackageMap[packName]; 
				if !exists {
					dep = newGoPackage(packName);
				}
				v.file.pack.depends.Push(dep);
			}

		}
		return nil;
	case *ast.Package:
		return v;
	case *ast.File:
		return v;
	case *ast.BadDecl:
		return v;
	case *ast.GenDecl:
		return v;
	case *ast.Ident:
		return nil;
	case *ast.FuncDecl:
		if n.Recv == nil && n.Name.Value == "main" && v.file.pack.name == "main" {
			v.file.hasMain = true;
		}
		return nil;
	default:
		return nil;
	}

	return nil; // unreachable
}

// ========== (local) functions ==========

/*
 readFiles reads all files with the .go extension and creates their AST.
 It also creates a list of local imports (everything starting with ./)
 and searches the main package files for the main function.
*/
func readFiles(rootpath string) {
	// path walker error channel
	errorChannel := make(chan os.Error, 64);

	// visitor for the path walker
	visitor := &goFileVisitor{};
	
	info("Parsing go file(s)...\n");
	
	path.Walk(rootpath, visitor, errorChannel);
	
	if err, ok := <-errorChannel; ok {
		error("Error while traversing directories: %s\n", err);
	}

	info("Read %d go file(s).\n", goFileVector.Len());
}

/*
 The compile method will run the compiler for every package it has found,
 starting with the main package.
*/
func compile(pack *goPackage) {
	// check for recursive dependencies
	if pack.inProgress {
		error("Found a recurisve dependency in %s. This is not supported in Go, aborting compilation.\n", pack.name);
		os.Exit(1);
	}
	pack.inProgress = true;

	// first compile all dependencies
	pack.depends.Do(func(e interface{}) {
		dep := e.(*goPackage);
		if !dep.compiled {
			compile(dep);
		}
	});

	// check if this package has any files (if not -> error)
	if pack.files.Len() == 0 {
		error("No files found for package %s.\n", pack.name);
		os.Exit(1);
	}
	
	// construct compiler command line arguments
	info("Compiling %s...\n", pack.name);
	argv := make([]string, pack.files.Len() + 3);

	argv[0] = compilerBin;

	argv[1] = "-o";

	switch os.Getenv("GOARCH") {
	case "amd64":
		argv[2] = pack.name + ".6";
	case "386":
		argv[2] = pack.name + ".8";
	case "arm":
		argv[2] = pack.name + ".5";
	}

	info("\tfiles: ");
	var j int = 3;
	for i := 0; i < pack.files.Len(); i++  {
		// "main" package is a special case, only include one file with main function
		gf := pack.files.At(i).(*goFile);
		if pack.name != "main" ||
			!gf.hasMain || gf.filename == mainGoFileName {
			argv[j] = gf.filename;
			info("%s ", argv[j]);
			j++;
		}
	}
	info("\n");
		
	cmd, err := exec.Run(compilerBin, argv[0:j], os.Environ(), exec.DevNull, 
		exec.PassThrough, exec.PassThrough);
	if err != nil {
		error("%s\n", err);
		os.Exit(1);
	}

	waitmsg, err := cmd.Wait(0);
	if err != nil {
		error("Compiler execution error (%s), aborting compilation.\n", err);
		os.Exit(1);
	}

	if waitmsg.ExitStatus() != 0 {
		compileError = true;
		pack.hasErrors = true;
	}
	
	// it should now be compiled
	pack.compiled = true;
	pack.inProgress = false;

}

/*
 Calls the linker for the main file, which should be called "main.*".
*/
func link() {
	argv := []string{
		linkerBin,
		"-o",
		mainGoFileName[0:len(mainGoFileName)-3],
		""};

	switch os.Getenv("GOARCH") {
	case "amd64":
		argv[3] = "main.6";
	case "386":
		argv[3] = "main.8";
	case "arm":
		argv[3] = "main.5";
	}
	

	// replace default output file name with command line parameter -o
	if *outputFileName != "" {
		argv[2] = *outputFileName;
	}

	info("Linking %s...\n", argv[2]);

	cmd, err := exec.Run(linkerBin, argv, os.Environ(), exec.DevNull, exec.PassThrough, exec.PassThrough);
	if err != nil {
		error("%s\n", err);
		os.Exit(1);
	}
	waitmsg, err := cmd.Wait(0);
	if err != nil {
		error("Linker execution error (%s), aborting compilation.\n", err);
		os.Exit(1);
	}

	if waitmsg.ExitStatus() != 0 {
		linkError = true;
	}
}


/*
 Prints debug messages. Same syntax as fmt.Printf.
*/
func debug(format string, v ...) {
	if *verboseMode && !*quietMode && !*quieterMode {
		fmt.Printf("DEBUG: ");
		fmt.Printf(format, v);
	}
}

/*
 Prints a debug message if enabled but without the "DEBUG: " prefix.
 Same syntax as fmt.Printf.
*/
func debugContinue(format string, v ...) {
	if *verboseMode && !*quietMode && !*quieterMode {
		fmt.Printf("       ");
		fmt.Printf(format, v);
	}
}

/*
 Prints an info message if enabled. This is for general feedback about what
 gobuild is currently doing. Same syntax as fmt.Printf.
*/
func info(format string, v ...) {
	if !*quietMode && !*quieterMode {
		//fmt.Print("INFO: ");
		fmt.Printf(format, v);
	}
}


/*
 Prints a warning if warnings are enabled. Same syntax as fmt.Printf.
*/
func warn(format string, v ...) {
	if !*quieterMode {
		fmt.Print("WARNING: ");
		fmt.Printf(format, v);
	}
}

/*
 Prints a warning message if enabled but without the "WARNING: " prefix.
 Same syntax as fmt.Printf.
*/
func warnContinue(format string, v ...) {
	if !*quieterMode {
		fmt.Print("         ");
		fmt.Printf(format, v);
	}
}

/*
 Prints an error message. Same syntax as fmt.Printf.
*/
func error(format string, v ...) {
	fmt.Fprint(os.Stderr, "ERROR: ");
	fmt.Fprintf(os.Stderr, format, v);
}

/*
 Prints an error message but without the "ERROR: " prefix.
 Same syntax as fmt.Printf.
*/
func errorContinue(format string, v ...) {
	fmt.Fprint(os.Stderr, "       ");
	fmt.Fprintf(os.Stderr, format, v);
}

// Returns the bigger number.
func max(a, b int) int {
	if a > b {
		return a;
	}
	return b;
}

func main() {
	var err os.Error;

	// parse command line arguments
	flag.Parse();
	
	// check if there's a main file parameter on the command line
	if flag.NArg() > 0 {
		mainGoFileName = flag.Arg(0);
		// and if the file actually exists
		f, err := os.Open(mainGoFileName, os.O_RDONLY, 0);
		if err != nil {
			error("Failed to read %s: %s\n", mainGoFileName, err);
			os.Exit(1);
		}
		f.Close();
	} else {
		*includeAllMain = true;
	}

	// get the compiler/linker executable
	switch os.Getenv("GOARCH") {
	case "amd64":
		compilerBin = "6g";
		linkerBin = "6l";
	case "386":
		compilerBin = "8g";
		linkerBin = "8l";
	case "arm":
		compilerBin = "5g";
		linkerBin = "5l";
	default:
		error("Please specify a valid GOARCH (amd64/386/arm).\n");
		os.Exit(1);		
	}

	// get the complete path to the compiler/linker
	compilerBin, err = exec.LookPath(compilerBin);
	if err != nil {
		error("Error: could not find compiler %s.\n", compilerBin);
		os.Exit(1);
	}
	linkerBin, err = exec.LookPath(linkerBin);
	if err != nil {
		error("Error: could not find linker %s.\n", linkerBin);
		os.Exit(1);
	}
	
	// get the root path from where the application was called
	rootPath, err = os.Getwd();
	if err != nil {
		error("Could not get the root path: %s\n", err);
		os.Exit(1);
	}

	// create the package map
	goPackageMap = make(map[string] *goPackage);

	// read all go files in the current path + subdirectories and parse them
	readFiles(rootPath);

	// check if there's a main package:
	mainPack, mainExists := goPackageMap["main"];
	if !mainExists {
		error("No main package found.\n");
		os.Exit(1);
	}
	
	// find the main function (if not specified in the command line)
	if mainGoFileName == "" {
		multiFound := false;
		mainPack.files.Do(func(e interface{}){
			gf := e.(*goFile);
			if gf.hasMain {
				if mainGoFileName == "" && !multiFound {
					mainGoFileName = gf.filename;
				} else if !multiFound {
					error("multiple files with main methods found.\n");
					errorContinue("Please specify one of the following as command line parameter:\n");
					errorContinue("\t %s\n", mainGoFileName);
					errorContinue("\t %s\n", gf.filename);
					multiFound = true;
				} else {
					errorContinue("\t %s\n", gf.filename);
				}
			}
		});
		if multiFound {
			os.Exit(1);
		}
	}

	// compile all needed packages
	compile(mainPack);

	// link everything together
	if !compileError {
		link();
	} else {
		error("Can't link executable because of compile errors.\n");
	}
}

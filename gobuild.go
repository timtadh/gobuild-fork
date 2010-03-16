// Copyright 2009-2010 by Maurice Gilden. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
 gobuild - build tool to automate building go programs/libraries
*/
package main

import (
	os "os"
	"exec"
	"flag"
	"path"
	"strings"
	"container/vector"
	"./godata"
	"./logger"
)

// ========== command line parameters ==========

var flagLibrary *bool = flag.Bool("lib", false, "build all packages as librarys")
var flagBuildAll *bool = flag.Bool("a", false, "build all executables")
var flagTesting *bool = flag.Bool("t", false, "(not yet implemented) Build all tests")
var flagSingleMainFile *bool = flag.Bool("single-main", false, "one main file per executable")
var flagIncludeInvisible *bool = flag.Bool("include-hidden", false, "Include hidden directories")
var flagOutputFileName *string = flag.String("o", "", "output file")
var flagQuietMode *bool = flag.Bool("q", false, "only print warnings/errors")
var flagQuieterMode *bool = flag.Bool("qq", false, "only print errors")
var flagVerboseMode *bool = flag.Bool("v", false, "print debug messages")
var flagIncludePaths *string = flag.String("I", "", "additional include paths")
var flagClean *bool = flag.Bool("clean", false, "delete all temporary files")
var flagRunExec *bool = flag.Bool("run", false, "run the created executable(s)")
var flagMatch *string = flag.String("match", "", "regular expression to select tests to run")
var flagBenchmarks *string = flag.String("benchmarks", "", "regular expression to select benchmarks to run")

// ========== global (package) variables ==========

var compilerBin string
var linkerBin string
var gopackBin string = "gopack"
var compileErrors bool = false
var linkErrors bool = false
var rootPath string
var rootPathPerm int
var objExt string
var outputDirPrefix string
var goPackages *godata.GoPackageContainer

// ========== goFileVisitor ==========

// this visitor looks for files with the extension .go
type goFileVisitor struct{
	rootpath string
	realpath string
	symname string
}


// implementation of the Visitor interface for the file walker
func (v *goFileVisitor) VisitDir(dirpath string, d *os.Dir) bool {
	if strings.LastIndex(dirpath, "/") < len(dirpath)-1 {
		if dirpath[strings.LastIndex(dirpath, "/")+1] == '.' {
			return *flagIncludeInvisible
		}
	}
	return true
}

// implementation of the Visitor interface for the file walker
func (v *goFileVisitor) VisitFile(filepath string, d *os.Dir) {
	// parse hidden directories?
	if (filepath[strings.LastIndex(filepath, "/")+1] == '.') && (!*flagIncludeInvisible) {
		return
	}

	// check if this is a symlink
	if dir, err := os.Stat(filepath); err == nil {
		if dir.FollowedSymlink && dir.IsDirectory() {
			readFiles(filepath)
		}
	} else {
		logger.Warn("%s\n", err);
	}

	if strings.HasSuffix(filepath, ".go") {
		// include *_test.go files?
		if strings.HasSuffix(filepath, "_test.go") && (!*flagTesting) {
			return
		}

		var gf godata.GoFile
		if v.realpath != v.rootpath {
			gf = godata.GoFile{v.symname + filepath[strings.LastIndex(filepath, "/"):],
				nil, false, false, strings.HasSuffix(filepath, "_test.go"), nil, nil,
			}
		} else {
			gf = godata.GoFile{filepath[len(v.realpath)+1 : len(filepath)], nil,
				false,false, strings.HasSuffix(filepath, "_test.go"), nil, nil,
			}
		}

		if gf.IsTestFile {
			gf.TestFunctions = new(vector.Vector)
			gf.BenchmarkFunctions = new(vector.Vector)
		}
		logger.Debug("Parsing file: %s\n", filepath)

		gf.ParseFile(goPackages)
	}
}

// ========== (local) functions ==========

// unused right now, may be used later for a target directory for .[568] files
func getObjDir() string {
	return ""

	// this doesn't work for 'import "./blub"' style imports
	/*
		if *flagTesting {
			return "_test/";
		}
		return "_obj/";*/
}


/*
 Returns an argv array in a single string with spaces dividing the entries.
*/
func getCommandline(argv []string) string {
	var str string
	for _, s := range argv {
		str += s + " "
	}
	return str[0 : len(str)-1]
}

/*
 readFiles reads all files with the .go extension and creates their AST.
 It also creates a list of local imports (everything starting with ./)
 and searches the main package files for the main function.
*/
func readFiles(rootpath string) {
	var realpath, symname string
	// path walker error channel
	errorChannel := make(chan os.Error, 64)

	// check if this is a symlink
	if dir, err := os.Stat(rootpath); err == nil {
		if dir.FollowedSymlink {
			realpath, _ = os.Readlink(rootpath)
			if realpath[0] != '/' {
				realpath = rootpath[0:strings.LastIndex(rootpath, "/")+1] + realpath
			}
			symname = rootpath[len(rootPath)+1:]
		} else {
			realpath = rootpath
		}
	} else {
		logger.Warn("%s\n", err)
	}

	// visitor for the path walker
	visitor := &goFileVisitor{rootpath, realpath, symname}

	path.Walk(visitor.realpath, visitor, errorChannel)

	if err, ok := <-errorChannel; ok {
		logger.Error("Error while traversing directories: %s\n", err)
	}
}

/*
 Creates a main package and _testmain.go file for building a test application.
*/
func createTestPackage() *godata.GoPackage {
	var testFileSource string
	var testArrays string
	var testCalls string
	var benchCalls string
	var testGoFile *godata.GoFile
	var testPack *godata.GoPackage
	var testFile *os.File
	var err os.Error
	var pack *godata.GoPackage
	var flagsDeleted bool = false

	testGoFile = new(godata.GoFile)
	testPack = godata.NewGoPackage("main")

	testGoFile.Filename = "_testmain.go"
	testGoFile.Pack = testPack
	testGoFile.HasMain = true
	testGoFile.IsTestFile = true

	testPack.OutputFile = "_testmain"
	testPack.Files.Push(testGoFile)

	// search for packages with _test.go files
	for _, packName := range goPackages.GetPackageNames() {
		pack, _ = goPackages.Get(packName)

		if pack.HasTestFiles() {
			testPack.Depends.Push(pack)
		}
	}

	if testPack.Depends.Len() == 0 {
		logger.Error("No _test.go files found.\n")
		os.Exit(1)
	}

	// imports
	testFileSource =
		"package main\n" +
			"\nimport \"testing\"\n" +
			"import \"fmt\"\n" +
			"import \"os\"\n"

	// will create an array per package with all the Test* and Benchmark* functions
	// tests/benchmarks will be done for each package seperatly so that running
	// the _testmain program will result in multiple PASS (or fail) outputs.
	for ipack := range testPack.Depends.Iter() {
		var tmpStr string
		var fnCount int = 0
		pack := (ipack.(*godata.GoPackage))
		
		// localPackVarName: contains the test functions, package name
		// with '/' replaced by '_'
		var localPackVarName string = strings.Map(func(rune int) int {
			if rune == '/' {
				return '_'
			}
			return rune;}, pack.Name)
		// localPackName: package name without path/parent directories
		var localPackName string
		if strings.LastIndex(pack.Name, "/") >= 0 {
			localPackName = pack.Name[strings.LastIndex(pack.Name, "/")+1:]
		} else {
			localPackName = pack.Name
		}

		testFileSource += "import \"" + pack.Name + "\"\n"

		tmpStr = "var test_" + localPackVarName + " = []testing.Test {\n"
		for igf := range pack.Files.Iter() {
			logger.Debug("Test* from %s: \n", (igf.(*godata.GoFile)).Filename)
			if (igf.(*godata.GoFile)).IsTestFile {
				for istr := range (igf.(*godata.GoFile)).TestFunctions.Iter() {
					tmpStr += "\ttesting.Test{ \"" +
						pack.Name + "." + istr.(string) +
						"\", " +
						localPackName + "." + istr.(string) +
						" },\n"
					fnCount++
				}
			}
		}
		tmpStr += "}\n\n"

		if fnCount > 0 {
			testCalls +=
				"\tfmt.Println(\"Testing " + pack.Name + ":\");\n" +
					"\ttesting.Main(test_" + localPackVarName + ");\n"
			testArrays += tmpStr
			
			if !flagsDeleted {
				// this is needed because testing.Main calls flags.Parse
				// which collides with previous calls to that function
				testCalls += "\tos.Args = []string{}\n"
				flagsDeleted = true
			}
		}

		fnCount = 0
		tmpStr = "var bench_" + localPackVarName + " = []testing.Benchmark {\n"
		for igf := range pack.Files.Iter() {
			if (igf.(*godata.GoFile)).IsTestFile {
				for istr := range (igf.(*godata.GoFile)).BenchmarkFunctions.Iter() {
					tmpStr += "\ttesting.Benchmark{ \"" +
						pack.Name + "." + istr.(string) +
						"\", " +
						localPackName + "." + istr.(string) +
						" },\n"
					fnCount++
				}
			}
		}
		tmpStr += "}\n\n"

		if fnCount > 0 {
			benchCalls +=
				"\tfmt.Println(\"Benchmarking " + pack.Name + ":\");\n" +
					"\ttesting.RunBenchmarks(bench_" + localPackVarName + ");\n"
			testArrays += tmpStr
		}
	}

	testFileSource += "\n" + testArrays

	// func main()
	testFileSource +=
		"\nfunc main() {\n" +
			testCalls +
			benchCalls +
			"}\n"

	testFile, err = os.Open(testGoFile.Filename, os.O_CREAT|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		logger.Error("Could not create %s: %s\n", testGoFile.Filename, err)
		os.Exit(1)
	}
	testFile.WriteString(testFileSource)

	testFile.Close()

	return testPack
}

/*
 The compile method will run the compiler for every package it has found,
 starting with the main package.
 Returns true if compiled successfully.
*/
func compile(pack *godata.GoPackage) bool {
	var argc int
	var argv []string
	var argvFilled int
	var objDir = "" //outputDirPrefix + getObjDir();

	// check for recursive dependencies
	if pack.InProgress {
		logger.Error("Found a recurisve dependency in %s. This is not supported in Go.\n", pack.Name)
		pack.HasErrors = true
		pack.InProgress = false
		return false
	}

	pack.InProgress = true

	// first compile all dependencies
	for idep := range pack.Depends.Iter() {
		dep := idep.(*godata.GoPackage)
		if dep.HasErrors {
			pack.HasErrors = true
			pack.InProgress = false
			return false
		}

		if !dep.Compiled &&
			(dep.Type == godata.LOCAL_PACKAGE ||
				dep.Type == godata.UNKNOWN_PACKAGE && dep.Files.Len() > 0) {
			if !compile(dep) {
				pack.HasErrors = true
				pack.InProgress = false
				return false
			}
		}
	}

	// cgo files (the ones which import "C") can't be compiled
	// at the moment. They need to be compiled by hand into .a files.
	if pack.HasCGOFiles() {
		if pack.HasExistingAFile() {
			pack.Compiled = true
			pack.InProgress = false
			return true
		} else {
			logger.Error("Can't compile cgo files. Please manually compile them.\n")
			os.Exit(1)
		}
	}

	// check if this package has any files (if not -> error)
	if pack.Files.Len() == 0 && pack.Type == godata.LOCAL_PACKAGE {
		logger.Error("No files found for package %s.\n", pack.Name)
		os.Exit(1)
	}

	// if the outputDirPrefix points to something, subdirectories 
	// need to be created if they don't already exist
	outputFile := objDir + pack.OutputFile
	if strings.Index(outputFile, "/") != -1 {
		path := outputFile[0:strings.LastIndex(outputFile, "/")]
		dir, err := os.Stat(path)
		if err != nil {
			err = os.MkdirAll(path, rootPathPerm)
			if err != nil {
				logger.Error("Could not create output path %s: %s\n", path, err)
				os.Exit(1)
			}
		} else if !dir.IsDirectory() {
			logger.Error("File found in %s instead of a directory.\n", path)
			os.Exit(1)
		}
	}

	// before compiling, remove any .a file
	// this is done because the compiler/linker looks for .a files
	// before it looks for .[568] files
	if err := os.Remove(outputFile + ".a"); err == nil {
		logger.Debug("Removed file %s.a.\n", outputFile)
	}

	// construct compiler command line arguments
	if pack.Name != "main" {
		logger.Info("Compiling %s...\n", pack.Name)
	} else {
		logger.Info("Compiling %s (%s)...\n", pack.Name, pack.OutputFile)
	}

	argc = pack.Files.Len() + 3
	if *flagIncludePaths != "" {
		argc += 2
	}
	if pack.NeedsLocalSearchPath() || objDir != "" {
		argc += 2
	}
	argv = make([]string, argc)

	argv[argvFilled] = compilerBin
	argvFilled++
	argv[argvFilled] = "-o"
	argvFilled++
	argv[argvFilled] = outputFile + objExt
	argvFilled++

	if *flagIncludePaths != "" {
		argv[argvFilled] = "-I"
		argvFilled++
		argv[argvFilled] = *flagIncludePaths
		argvFilled++
	}

	if pack.NeedsLocalSearchPath() || objDir != "" {
		argv[argvFilled] = "-I"
		argvFilled++
		if objDir != "" {
			argv[argvFilled] = objDir
		} else {
			argv[argvFilled] = "."
		}
		argvFilled++
	}

	for i := 0; i < pack.Files.Len(); i++ {
		gf := pack.Files.At(i).(*godata.GoFile)
		argv[argvFilled] = gf.Filename
		argvFilled++
	}

	logger.Debug("%s\n", getCommandline(argv[0:argvFilled]))
	cmd, err := exec.Run(compilerBin, argv[0:argvFilled], os.Environ(), rootPath,
		exec.DevNull, exec.PassThrough, exec.PassThrough)
	if err != nil {
		logger.Error("%s\n", err)
		os.Exit(1)
	}

	waitmsg, err := cmd.Wait(0)
	if err != nil {
		logger.Error("Compiler execution error (%s), aborting compilation.\n", err)
		os.Exit(1)
	}

	if waitmsg.ExitStatus() != 0 {
		pack.HasErrors = true
		pack.InProgress = false
		return false
	}

	// it should now be compiled
	pack.Compiled = true
	pack.InProgress = false

	return true
}

/*
 Calls the linker for the main file, which should be called "main.(5|6|8)".
*/
func link(pack *godata.GoPackage) bool {
	var argc int
	var argv []string
	var argvFilled int
	var objDir string = "" //outputDirPrefix + getObjDir();

	// build the command line for the linker
	argc = 4
	if *flagIncludePaths != "" {
		argc += 2
	}
	if pack.NeedsLocalSearchPath() {
		argc += 2
	}

	argv = make([]string, argc)

	argv[argvFilled] = linkerBin
	argvFilled++
	argv[argvFilled] = "-o"
	argvFilled++
	argv[argvFilled] = outputDirPrefix + pack.OutputFile
	argvFilled++
	if *flagIncludePaths != "" {
		argv[argvFilled] = "-L"
		argvFilled++
		argv[argvFilled] = *flagIncludePaths
		argvFilled++
	}
	if pack.NeedsLocalSearchPath() {
		argv[argvFilled] = "-L"
		argvFilled++
		if objDir != "" {
			argv[argvFilled] = objDir
		} else {
			argv[argvFilled] = "."
		}
		argvFilled++
	}
	argv[argvFilled] = objDir + pack.OutputFile + objExt
	argvFilled++

	logger.Info("Linking %s...\n", argv[2])
	logger.Debug("%s\n", getCommandline(argv))

	cmd, err := exec.Run(linkerBin, argv, os.Environ(), rootPath,
		exec.DevNull, exec.PassThrough, exec.PassThrough)
	if err != nil {
		logger.Error("%s\n", err)
		os.Exit(1)
	}
	waitmsg, err := cmd.Wait(0)
	if err != nil {
		logger.Error("Linker execution error (%s), aborting compilation.\n", err)
		os.Exit(1)
	}

	if waitmsg.ExitStatus() != 0 {
		logger.Error("Linker returned with errors, aborting.\n")
		return false
	}
	return true
}


/*
 Executes something. Used for the -run command line option.
*/
func runExec(argv []string) {
	logger.Info("Executing %s:\n", argv[0]);
	logger.Debug("%s\n", getCommandline(argv))
	cmd, err := exec.Run(argv[0], argv, os.Environ(), rootPath,
		exec.PassThrough, exec.PassThrough, exec.PassThrough)
	if err != nil {
		logger.Error("%s\n", err)
		os.Exit(1)
	}
	waitmsg, err := cmd.Wait(0)
	if err != nil {
		logger.Error("Executing %s failed: %s.\n", argv[0], err)
		os.Exit(1)
	}

	if waitmsg.ExitStatus() != 0 {
		os.Exit(waitmsg.ExitStatus());
	}
}


/*
 Creates a .a file for a single GoPackage
*/
func packLib(pack *godata.GoPackage) {
	var objDir string = "" //outputDirPrefix + getObjDir();

	// ignore packages that need to be build manually (like cgo packages)
	if pack.HasCGOFiles() {
		logger.Debug("Skipped %s.a because it can't be build with gobuild.\n", pack.Name)
		return
	}

	logger.Info("Creating %s.a...\n", pack.Name)

	argv := []string{
		gopackBin,
		"crg", // create new go archive
		outputDirPrefix + pack.Name + ".a",
		objDir + pack.Name + objExt,
	}

	logger.Debug("%s\n", getCommandline(argv))
	cmd, err := exec.Run(gopackBin, argv, os.Environ(), rootPath,
		exec.DevNull, exec.PassThrough, exec.PassThrough)
	if err != nil {
		logger.Error("%s\n", err)
		os.Exit(1)
	}
	waitmsg, err := cmd.Wait(0)
	if err != nil {
		logger.Error("gopack execution error (%s), aborting.\n", err)
		os.Exit(1)
	}

	if waitmsg.ExitStatus() != 0 {
		logger.Error("gopack returned with errors, aborting.\n")
		os.Exit(waitmsg.ExitStatus())
	}
}


/*
 Build an executable from the given sources.
*/
func buildExecutable() {
	var executables []string
	var execFilled int

	// check if there's a main package:
	if goPackages.GetMainCount() == 0 {
		logger.Error("No main package found.\n")
		os.Exit(1)
	}

	// multiple main, no command file from command line and no -a -> error
	if (goPackages.GetMainCount() > 1) && (flag.NArg() == 0) && !*flagBuildAll {
		logger.Error("Multiple files found with main function.\n")
		logger.ErrorContinue("Please specify one or more as command line parameter or\n")
		logger.ErrorContinue("run gobuild with -a. Available main files are:\n")
		for _, fn := range goPackages.GetMainFilenames() {
			logger.ErrorContinue("\t %s\n", fn)
		}
		os.Exit(1)
	}

	// compile all needed packages
	if flag.NArg() > 0 {
		if *flagRunExec {
			executables = make([]string, flag.NArg())
		}
		for _, fn := range flag.Args() {
			mainPack, exists := goPackages.GetMain(fn, !*flagSingleMainFile)
			if !exists {
				logger.Error("File %s not found.\n", fn)
				return // or os.Exit?
			}

			if compile(mainPack) {
				// link everything together
				if link(mainPack) {
					if *flagRunExec {
						executables[execFilled] = outputDirPrefix + mainPack.OutputFile
						execFilled++
					}
				} else {
					linkErrors = true
				}
			} else {
				logger.Error("Can't link executable because of compile errors.\n")
				compileErrors = true
			}
		}
	} else {
		if *flagRunExec {
			executables = make([]string, goPackages.GetMainCount())
		}
		for _, mainPack := range goPackages.GetMainPackages(!*flagSingleMainFile) {

			if compile(mainPack) {
				if link(mainPack) {
					if *flagRunExec {
						executables[execFilled] = outputDirPrefix + mainPack.OutputFile
						execFilled++
					}
				} else {
					linkErrors = true
				}
			} else {
				logger.Error("Can't link executable because of compile errors.\n")
				compileErrors = true
			}
		}
	}

	if *flagRunExec && !linkErrors && !compileErrors {
		for i := 0; i < execFilled; i++ {
			runExec([]string{executables[i]})
		}
	}
}


/*
 Build library files (.a) for all packages or the ones given though
 command line parameters.
*/
func buildLibrary() {
	var packNames []string
	var pack *godata.GoPackage
	var exists bool

	if goPackages.GetPackageCount() == 0 {
		logger.Warn("No packages found to build.\n")
		return
	}

	// check for the first package that could be build with gobuild
	var hasNoCompilablePacks bool = true
	for _, packName := range goPackages.GetPackageNames() {
		pack, _ := goPackages.Get(packName)
		if pack.Name == "main" {
			continue
		}
		if pack.Files.Len() > 0 && !pack.HasCGOFiles() {
			hasNoCompilablePacks = false
			break
		}
	}
	if hasNoCompilablePacks {
		logger.Warn("No packages found that could be compiled by gobuild.\n")
		return
	}

	// check for command line parameters
	if flag.NArg() > 0 {
		packNames = flag.Args()
	} else {
		packNames = goPackages.GetPackageNames()
	}


	// loop over all packages, compile them and build a .a file
	for _, name := range packNames {

		if name == "main" {
			continue // don't make this into a library
		}

		pack, exists = goPackages.Get(name)
		if !exists {
			logger.Error("Package %s doesn't exist.\n", name)
			continue // or exit?
		}

		// don't compile remote packages or packages without files
		if pack.Type == godata.REMOTE_PACKAGE || pack.Files.Len() == 0 {
			continue
		}

		// these packages come from invalid/unhandled imports
		if pack.Files.Len() == 0 {
			logger.Debug("Skipping package %s, no files to compile.\n", pack.Name)
			continue
		}

		if !pack.Compiled && !pack.HasErrors {
			compileErrors = !compile(pack) || compileErrors
		}

		if pack.HasErrors {
			logger.Error("Can't create library because of compile errors.\n")
			compileErrors = true
		} else {
			packLib(pack)
		}
	}
}

/*
 Creates a new file called _testmain.go and compiles/links it to _testmain.
 If the -run command line option is given it will also run the tests. In this
 case -benchmarks/-match/-v are also passed on.
*/
func buildTestExecutable() {
	// this will create a file called "_testmain.go"
	testPack := createTestPackage()

	if compile(testPack) {
		linkErrors = !link(testPack) || linkErrors
	} else {
		logger.Error("Can't link executable because of compile errors.\n")
		compileErrors = true
	}

	// delete temporary _testmain.go file
	os.Remove("_testmain.go")

	if compileErrors || linkErrors {
		return
	}

	if *flagRunExec {
		var argvFilled int
		var argc int = 1
		if *flagMatch != "" {
			argc += 2
		}
		if *flagBenchmarks != "" {
			argc += 2
		}
		if *flagVerboseMode {
			argc++
		}
		argv := make([]string, argc)
		argv[argvFilled] = outputDirPrefix + testPack.OutputFile
		argvFilled++
		if *flagMatch != "" {
		        argv[argvFilled] = "-match"
			argvFilled++
			argv[argvFilled] = *flagMatch
			argvFilled++
		}
		if *flagBenchmarks != "" {
		        argv[argvFilled] = "-benchmarks"
			argvFilled++
			argv[argvFilled] = *flagBenchmarks
			argvFilled++
		}
		if *flagVerboseMode {
			argv[argvFilled] = "-v"
			argvFilled++
		}

		runExec(argv)
	}
}

/*
 This function does exactly the same as "make clean".
*/
func clean() {
	bashBin, err := exec.LookPath("bash")
	if err != nil {
		logger.Error("Need bash to clean.\n")
		os.Exit(127)
	}

	argv := []string{bashBin, "-c", "commandhere"}

	if *flagVerboseMode {
		argv[2] = "rm -rfv *.[568]"
	} else {
		argv[2] = "rm -rf *.[568]"
	}

	logger.Info("Running: %v\n", argv[2:])

	cmd, err := exec.Run(bashBin, argv, os.Environ(), rootPath,
		exec.DevNull, exec.PassThrough, exec.PassThrough)
	if err != nil {
		logger.Error("%s\n", err)
		os.Exit(1)
	}
	waitmsg, err := cmd.Wait(0)
	if err != nil {
		logger.Error("Couldn't delete files: %s\n", err)
		os.Exit(1)
	}

	if waitmsg.ExitStatus() != 0 {
		logger.Error("rm returned with errors.\n")
		os.Exit(waitmsg.ExitStatus())
	}
}

/*
 Allows *.a files to be OK'd by Gobuild, so they can be used as local packages
 without requiring the source code to be compiled.
*/
func markCompiledPackages() {
	for _, packName := range goPackages.GetPackageNames() {
		pack, _ := goPackages.Get(packName)
		if pack.HasExistingAFile() {
			pack.Compiled = true
		}
	}
}

// Returns the bigger number.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

/*
 Entry point. Used for setting some variables and parsing the command line.
*/
func main() {
	var err os.Error
	var rootPathDir *os.Dir

	// parse command line arguments
	flag.Parse()

	if *flagQuieterMode {
		logger.SetVerbosityLevel(logger.ERROR)
	} else if *flagQuietMode {
		logger.SetVerbosityLevel(logger.WARN)
	} else if *flagVerboseMode {
		logger.SetVerbosityLevel(logger.DEBUG)
	}

	if *flagClean {
		clean()
		os.Exit(0)
	}

	// get the compiler/linker executable
	switch os.Getenv("GOARCH") {
	case "amd64":
		compilerBin = "6g"
		linkerBin = "6l"
		objExt = ".6"
	case "386":
		compilerBin = "8g"
		linkerBin = "8l"
		objExt = ".8"
	case "arm":
		compilerBin = "5g"
		linkerBin = "5l"
		objExt = ".5"
	default:
		logger.Error("Please specify a valid GOARCH (amd64/386/arm).\n")
		os.Exit(1)
	}

	// get the complete path to the compiler/linker
	compilerBin, err = exec.LookPath(compilerBin)
	if err != nil {
		logger.Error("Could not find compiler %s: %s\n", compilerBin, err)
		os.Exit(127)
	}
	linkerBin, err = exec.LookPath(linkerBin)
	if err != nil {
		logger.Error("Could not find linker %s: %s\n", linkerBin, err)
		os.Exit(127)
	}
	gopackBin, err = exec.LookPath(gopackBin)
	if err != nil {
		logger.Error("Could not find gopack executable (%s): %s\n", gopackBin, err)
		os.Exit(127)
	}

	// get the root path from where the application was called
	// and its permissions (used for subdirectories)
	if rootPath, err = os.Getwd(); err != nil {
		logger.Error("Could not get the root path: %s\n", err)
		os.Exit(1)
	}
	if rootPathDir, err = os.Stat(rootPath); err != nil {
		logger.Error("Could not read the root path: %s\n", err)
		os.Exit(1)
	}
	rootPathPerm = rootPathDir.Permission()

	// create the package container
	goPackages = godata.NewGoPackageContainer()

	// check if -o with path
	if *flagOutputFileName != "" {
		dir, err := os.Stat(*flagOutputFileName)
		if err != nil {
			// doesn't exist? try to make it if it's a path
			if (*flagOutputFileName)[len(*flagOutputFileName)-1] == '/' {
				err = os.MkdirAll(*flagOutputFileName, rootPathPerm)
				if err == nil {
					outputDirPrefix = *flagOutputFileName
				}
			} else {
				godata.DefaultOutputFileName = *flagOutputFileName
			}
		} else if dir.IsDirectory() {
			if (*flagOutputFileName)[len(*flagOutputFileName)-1] == '/' {
				outputDirPrefix = *flagOutputFileName
			} else {
				outputDirPrefix = *flagOutputFileName + "/"
			}
		} else {
			godata.DefaultOutputFileName = *flagOutputFileName
		}

		// make path to output file
		if outputDirPrefix == "" && strings.Index(*flagOutputFileName, "/") != -1 {
			err = os.MkdirAll((*flagOutputFileName)[0:strings.LastIndex(*flagOutputFileName, "/")], rootPathPerm)
			if err != nil {
				logger.Error("Could not create %s: %s\n",
					(*flagOutputFileName)[0:strings.LastIndex(*flagOutputFileName, "/")],
					err)
			}
		}

	}

	// read all go files in the current path + subdirectories and parse them
	logger.Info("Parsing go file(s)...\n")
	readFiles(rootPath)

	if *flagTesting {
		buildTestExecutable()
	} else if *flagLibrary {
		buildLibrary()
	} else {
		buildExecutable()
	}

	// make sure exit status is != 0 if there were compiler/linker errors
	if compileErrors || linkErrors {
		os.Exit(1)
	}
}

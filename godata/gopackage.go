// Copyright 2009-2010 by Maurice Gilden. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
 Code for handling go packages.
*/
package godata

import "container/vector"
import "os"
import "./logger"


const (
	UNKNOWN_PACKAGE = iota // could be in the local path or somewhere else
	                       // it's local if the package has files
	LOCAL_PACKAGE   // imported with "./name" (and not "name")
	REMOTE_PACKAGE  // unused right now
)

// ================================
// ========== GoPackage ===========
// ================================

type GoPackage struct {
	Name       string         // name of the package
	Path       string         // possible relative path to the package
	Type       int            // local, remote or unknown (default)
	Files      *vector.Vector  // a list of files for this package
	Depends    *vector.Vector  // a list of other local packages this one depends on
	Compiled   bool           // true = finished compiling
	InProgress bool           // true = currently trying to compile dependencies (needed to find recursive dependencies)
	HasErrors  bool           // true = compiler returned an error
	OutputFile string         // filename (and maybe path) of the output files without extensions
}

/*
 Creates a new goPackage.
*/
func NewGoPackage(name string) *GoPackage {
	pack := new(GoPackage)
	pack.Type = UNKNOWN_PACKAGE
	pack.Compiled = false
	pack.InProgress = false
	pack.HasErrors = false
	pack.Name = name
	pack.Files = new(vector.Vector)
	pack.Depends = new(vector.Vector)
	pack.OutputFile = name

	return pack
}

/*
 Creates a clone of a package. Entries in files and depends are the same but with new vectors.
*/
func (this *GoPackage) Clone() *GoPackage {
	pack := new(GoPackage)
	pack.Type = this.Type
	pack.Compiled = this.Compiled
	pack.InProgress = this.InProgress
	pack.HasErrors = this.HasErrors
	pack.Name = this.Name
	pack.Files = new(vector.Vector)
	this.Files.Do(func(gf interface{}) { pack.Files.Push(gf.(*GoFile)) })
	pack.Depends = new(vector.Vector)
	this.Depends.Do(func(dep interface{}) { pack.Depends.Push(dep.(*GoPackage)) })
	pack.OutputFile = this.OutputFile

	return pack
}

/*
 Merge a different package to this package. Will add dependencies and files, but
 keep all other things as they are.
*/
func (this *GoPackage) Merge(pack *GoPackage) {
	if this == pack {
		logger.Warn("Trying to merge identical packages!\n")
		return // don't merge duplicates
	}
	pack.Files.Do(func(gf interface{}) { this.Files.Push(gf.(*GoFile)) })
	pack.Depends.Do(func(dep interface{}) { this.Depends.Push(dep.(*GoPackage)) })
	if pack.Type == LOCAL_PACKAGE {
		this.Type = LOCAL_PACKAGE
	}
}

/*

*/
func (this *GoPackage) NeedsLocalSearchPath() bool {
	var ret bool = false
	this.Depends.Do(func(dep interface{}) {
		depPack := dep.(*GoPackage)
		if depPack.Type == UNKNOWN_PACKAGE && depPack.Files.Len() > 0 {
			ret = true
		}
	})

	return ret
}

/*
 Returns true if one of the files for this package contains some test functions.
*/
func (this *GoPackage) HasTestFiles() bool {
	for _, e := range *this.Files {
		if e.(*GoFile).IsTestFile {
			return true
		}
	}
	return false
}

/*
 Check files if one of them needs to be build with cgo. Those
 can't be compiled by gobuild right now.
*/
func (this *GoPackage) HasCGOFiles() bool {
	for _, e := range *this.Files {
		if e.(*GoFile).IsCGOFile {
			return true
		}
	}

	return false
}

/*
 This looks for an existing .a file for this package
 and returns true if one was found.
*/
func (this *GoPackage) HasExistingAFile() bool {
	_, err := os.Stat(this.OutputFile + ".a")
	return err == nil
}


// ================================
// ====== GoPackageContainer ======
// ================================

type GoPackageContainer struct {
	packages map[string]*GoPackage
	mains    map[string]*GoPackage
}

func NewGoPackageContainer() *GoPackageContainer {
	gpc := new(GoPackageContainer)
	gpc.packages = make(map[string]*GoPackage)
	gpc.mains = make(map[string]*GoPackage)
	return gpc
}

/*
 Will add a package to the list of packages. If there is already a package with
 the same name the new package will be merged to the old one. The returned
 package is the one that should be used after adding it.
*/
func (this *GoPackageContainer) AddPackage(pack *GoPackage) *GoPackage {
	if existingPack, exists := this.packages[pack.Name]; exists {
		if existingPack != pack {
			existingPack.Merge(pack)
		}
		return existingPack
	} else {
		this.packages[pack.Name] = pack
	}
	return pack
}

/*
 Creates an empty GoPackage and adds it to the container.
*/
func (this *GoPackageContainer) AddNewPackage(packName string) (pack *GoPackage) {
	pack = NewGoPackage(packName)
	pack = this.AddPackage(pack)
	return
}

/*
 Adds a GoFile to the list of packages. This will also create a new package
 if there is none yet. After this operation gf.Pack points to the package
 this file was added to.
*/
func (this *GoPackageContainer) AddFile(gf *GoFile, packageName string) {
	var exists bool
	var existingPack *GoPackage

	// main package file with main func is a special case
	// and needs to be put into this.mains
	if packageName == "main" && gf.HasMain {
		this.mains[gf.Filename] = gf.Pack

		// overwrite output name (main) with better one (-o <name> or filename)
		if DefaultOutputFileName == "" {
			gf.Pack.OutputFile = gf.Filename[0 : len(gf.Filename)-3]
		} else {
			gf.Pack.OutputFile = DefaultOutputFileName
		}
		gf.Pack.Files.Push(gf)
		return
	}

	// check if package is already known
	existingPack, exists = this.Get(packageName)
	if !exists {
		this.AddPackage(gf.Pack)
	} else if existingPack != gf.Pack {
		existingPack.Merge(gf.Pack)
		gf.Pack = existingPack
	}

	gf.Pack.Files.Push(gf)
}

/*
 Returns a GoPackage for a given package name, or (nil, false) if none was found.
*/
func (this *GoPackageContainer) Get(name string) (pack *GoPackage, exists bool) {
	pack, exists = this.packages[name]

	return
}

/*
 Will return the main package for a certain filename. That file must include
 a main function. If "merge" is true the returned package is a merge of the
 package with the main function file and all other files without main function
 that are in the main package.
 The returned package may be a copy of the one inside the container. Writing to
 it might not change values in the original package.
*/
func (this *GoPackageContainer) GetMain(filename string, merge bool) (pack *GoPackage, exists bool) {
	var mainPack *GoPackage

	// first check if file exists
	if pack, exists = this.mains[filename]; !exists {
		return nil, exists
	}

	// get the main package without main functions
	mainPack, exists = this.packages["main"]

	if exists && merge {
		pack = pack.Clone()
		pack.Merge(mainPack)
	}

	return pack, true
}

/*

*/
func (this *GoPackageContainer) GetPackageCount() int {
	return len(this.packages)
}

/*

*/
func (this *GoPackageContainer) GetMainCount() int {
	return len(this.mains)
}

/*

*/
func (this *GoPackageContainer) GetMainFilenames() (names []string) {
	names = make([]string, this.GetMainCount())
	var i int
	for fn, _ := range this.mains {
		names[i] = fn
		i++
	}

	return
}

/*
 This method will return an array of  packages, one for every file that has
 a main function in it. If merge is true the packages will be merged with other
 files of the main package that don't have a main function in it.
 The returned packages may be copies of the one inside the container. Writing to
 them might not change values in the original packages.
*/
func (this *GoPackageContainer) GetMainPackages(merge bool) (pack []*GoPackage) {
	pack = make([]*GoPackage, this.GetMainCount())
	mainPack, mainExists := this.packages["main"]
	var i int
	for _, p := range this.mains {
		pack[i] = p
		if merge && mainExists {
			pack[i] = pack[i].Clone()
			pack[i].Merge(mainPack)
		}

		i++
	}
	return
}

func (this *GoPackageContainer) GetPackageNames() (packNames []string) {
	var i int
	packNames = make([]string, len(this.packages))
	for name, _ := range this.packages {
		packNames[i] = name
		i++
	}
	return
}


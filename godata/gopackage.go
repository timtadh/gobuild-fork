// Copyright 2009 by Maurice Gilden. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
 Code for handling go packages.
*/
package godata

import "container/vector"
//import "./logger"

const (
	UNKNOWN_PACKAGE = iota;
	LOCAL_PACKAGE;
	REMOTE_PACKAGE;
)

// ================================
// ========== GoPackage ===========
// ================================

type GoPackage struct {
	Name string;             // name of the package
	Path string;             // possible relative path to the package
	Type int;                // local, remote or unknown (default)
	Files *vector.Vector;    // a list of files for this package
	Depends *vector.Vector;  // a list of other local packages this one depends on
	Compiled bool;           // true = finished compiling
	InProgress bool;         // true = currently trying to compile dependencies (needed to find recursive dependencies)
	HasErrors bool;          // true = compiler returned an error
	OutputFile string;       // filename (and maybe path) of the output files without extensions
}

/*
 Creates a new goPackage.
*/
func NewGoPackage(name string) *GoPackage {
	pack := new(GoPackage);
	pack.Type = UNKNOWN_PACKAGE;
	pack.Compiled = false;
	pack.InProgress = false;
	pack.HasErrors = false;
	pack.Name = name;
	pack.Files = new(vector.Vector);
	pack.Depends = new(vector.Vector);
	pack.OutputFile = name;
	
	return pack;
}

/*
 Creates a clone of a package. Entries in files and depends are the same but with new vectors.
*/
func (this *GoPackage) Clone() *GoPackage {
	pack := new(GoPackage);
	pack.Type = this.Type;
	pack.Compiled = this.Compiled;
	pack.InProgress = this.InProgress;
	pack.HasErrors = this.HasErrors;
	pack.Name = this.Name;
	pack.Files = new(vector.Vector);
	this.Files.Do(func(gf interface{}) {
		pack.Files.Push(gf.(*GoFile));
	});
	pack.Depends = new(vector.Vector);
	this.Depends.Do(func(dep interface{}) {
		pack.Depends.Push(dep.(*GoPackage));
	});
	pack.OutputFile = this.OutputFile;

	return pack;
}

/*
 Merge a different package to this package. Will add dependencies and files, but
 keep all other things as they are.
*/
func (this *GoPackage) Merge(pack *GoPackage) {
	pack.Files.Do(func(gf interface{}) {
		this.Files.Push(gf.(*GoFile));
	});
	pack.Depends.Do(func(dep interface{}) {
		this.Depends.Push(dep.(*GoPackage));
	});
}

// ================================
// ====== GoPackageContainer ======
// ================================

type GoPackageContainer struct {
	packages map[string]*GoPackage;
	mains map[string]*GoPackage;
}

func NewGoPackageContainer() *GoPackageContainer {
	gpc := new(GoPackageContainer);
	gpc.packages = make(map[string]*GoPackage);
	gpc.mains = make(map[string]*GoPackage);
	return gpc;
}

/*
 Will add a package to the list of packages. If there is already a package with
 the same name the new package will be merged to the old one.
*/
func (this *GoPackageContainer) AddPackage(pack *GoPackage) {
	if existingPack, exists := this.packages[pack.Name]; exists {
		existingPack.Merge(pack);
	} else {
		this.packages[pack.Name] = pack;
	}
}

/*
 Creates an empty GoPackage and adds it to the container.
*/
func (this *GoPackageContainer) AddNewPackage(packName string) (pack *GoPackage) {
	pack = NewGoPackage(packName);
	this.AddPackage(pack);
	return;
}

/*
 Adds a GoFile to the list of packages. This will also create a new package
 if there is none yet. After this operation gf.Pack points to the package
 this file was added to.
*/
func (this *GoPackageContainer) AddFile(gf *GoFile, packageName string) {
	var exists bool;

	// main package file with main func is a special case
	// and needs to be put into this.mains
	if packageName == "main" && gf.HasMain {
		if gf.Pack == nil {
			gf.Pack = NewGoPackage("main");
			gf.Pack.Files.Push(gf);
		}
		this.mains[gf.Filename] = gf.Pack;

		// overwrite output name (main) with better one (-o <name> or filename)
		if DefaultOutputFileName == "" {
			gf.Pack.OutputFile = gf.Filename[0:len(gf.Filename)-3];
		} else {
			gf.Pack.OutputFile = DefaultOutputFileName;
		}
		return;
	}

	// check if package is already known
	gf.Pack, exists = this.Get(packageName);
	
	if !exists {
		gf.Pack = NewGoPackage(packageName);
		this.AddPackage(gf.Pack);
	}
	gf.Pack.Files.Push(gf);
}

/*
 Returns a GoPackage for a given package name, or (nil, false) if none was found.
*/
func (this *GoPackageContainer) Get(name string) (pack *GoPackage, exists bool) {
	pack, exists = this.packages[name];

	return;
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
	var mainPack *GoPackage;
	
	// first check if file exists
	if pack, exists = this.mains[filename]; !exists {
		return nil, exists;
	}

	// get the main package without main functions
	mainPack, exists = this.packages["main"];
	
	if exists && merge {
		pack = pack.Clone();
		pack.Merge(mainPack);
	}

	return pack, true;
}

/*

*/
func (this *GoPackageContainer) GetPackageCount() int {
	return len(this.packages);
}

/*

*/
func (this *GoPackageContainer) GetMainCount() int {
	return len(this.mains);
}

/*

*/
func (this *GoPackageContainer) GetMainFilenames() (names []string) {
	names = make([]string, this.GetMainCount());
	var i int;
	for fn, _ := range this.mains {
		names[i] = fn;
		i++;
	}

	return;
}

/*
 This method will return an array of  packages, one for every file that has
 a main function in it. If merge is true the packages will be merged with other
 files of the main package that don't have a main function in it.
 The returned packages may be copies of the one inside the container. Writing to
 them might not change values in the original packages.
*/
func (this *GoPackageContainer) GetMainPackages(merge bool) (pack []*GoPackage) {
	pack = make([]*GoPackage, this.GetMainCount());
	mainPack, mainExists := this.packages["main"];
	var i int;
	for _, p := range this.mains {
		pack[i] = p;
		if merge && mainExists {
			pack[i] = pack[i].Clone();
			pack[i].Merge(mainPack);
		}
			
		i++;
	}
	return;
}

func (this *GoPackageContainer) GetPackageNames() (packNames []string) {
	var i int;
	packNames = make([]string, len(this.packages));
	for name, _ := range this.packages {
		packNames[i] = name;
		i++;
	}
	return;
}




# Copyright 2009 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

include $(GOROOT)/src/Make.$(GOARCH)

TARG=gobuild
GOFILES=gobuild.go

all: logger.6 godata.6 $(TARG)

logger.6: 
	$(GC) -o logger.$O logger/logger.go

godata.6:
	$(GC) -o godata.$O godata/gofile.go godata/gopackage.go


include $(GOROOT)/src/Make.cmd


# Copyright 2009 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

include $(GOROOT)/src/Make.$(GOARCH)

TARG=gobuild
GOFILES=gobuild.go
O_FILES=logger.$O godata.$O

all: $(O_FILES)
install: $(O_FILES)

include $(GOROOT)/src/Make.cmd

logger.$O:
	$(QUOTED_GOBIN)/$(GC) -o logger.$O logger/logger.go

godata.$O:
	$(QUOTED_GOBIN)/$(GC) -o godata.$O godata/gofile.go godata/gopackage.go


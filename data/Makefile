# Makefile for clone2fs
# Copyright (C) 2008 Michael Riepe
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation; either version 2 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301, USA

# @(#) $Id: Makefile,v 1.4 2010/01/31 23:21:23 michael Exp $

PACKAGE = clone2fs
VERSION = 1.3.0

sbindir = /sbin

CC = gcc
CFLAGS = -O2 -fno-strict-aliasing -g -Wall
CPPFLAGS = $(LIBE2FS_CFLAGS) $(LFS_CFLAGS) $(DEFS)
DEFS = -DVERSION_STRING=\"$(VERSION)\" $(HAVE_FSYNC)
LDFLAGS = $(LIBE2FS_LDFLAGS) $(LFS_LDFLAGS)
LDLIBS = $(LIBE2FS_LIBS) $(ZLIB_LIBS) $(LFS_LIBS)

HAVE_FSYNC = -DHAVE_FSYNC

LFS_CFLAGS = `getconf LFS_CFLAGS`
LFS_LDFLAGS = `getconf LFS_LDFLAGS`
LFS_LIBS = `getconf LFS_LIBS`

#LIBE2FS_CFLAGS = -I/usr/local/include
#LIBE2FS_LDFLAGS = -L/usr/local/lib
LIBE2FS_LIBS = -lext2fs -lcom_err

DISTFILES = clone2fs.c Makefile README NEWS COPYING

all: clone2fs

clone2fs: clone2fs.o

check:

install: all
	install -d $(sbindir)
	install -c clone2fs $(sbindir)

uninstall:
	rm -f $(sbindir)/clone2fs

mostlyclean:
	rm -f *.o

clean: mostlyclean
	rm -f clone2fs

distclean: clean

distdir = $(PACKAGE)-$(VERSION)

dist: $(distdir).tar.gz

$(distdir).tar.gz: $(DISTFILES)
	rm -rf $(distdir)
	mkdir $(distdir)
	ln $(DISTFILES) $(distdir)
	cd $(distdir) && \
		find . -type f ! -name MANIFEST -exec wc -c {} \; | \
		sed 's, \./, ,' | sort -k2 >MANIFEST
	rm -f $(distdir).tar.gz $(PACKAGE).tar.gz
	tar cvf - $(distdir) | gzip > $(distdir).tar.gz
	ln -fs $(distdir).tar.gz $(PACKAGE).tar.gz


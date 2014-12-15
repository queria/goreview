// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
)

var hookPath = ".git/hooks/"
var hookFiles = []string{
	"commit-msg",
}

func installHook() {
	for _, hookFile := range hookFiles {
		filename := filepath.Join(repoRoot(), hookPath+hookFile)

		// Special case: remove old commit-msg shell script
		// in favor of invoking the git-review hook implementation,
		// which will be easier to change in the future.
		if hookFile == "commit-msg" {
			data, err := ioutil.ReadFile(filename)
			if err == nil && string(data) == oldCommitMsgHook {
				verbosef("removing old commit-msg hook")
				os.Remove(filename)
			}
		}

		hookContent := fmt.Sprintf(hookScript, hookFile)

		// If hook file exists, assume it is okay.
		_, err := os.Stat(filename)
		if err == nil {
			if *verbose > 0 {
				data, err := ioutil.ReadFile(filename)
				if err != nil {
					verbosef("reading hook: %v", err)
				} else if string(data) != hookContent {
					verbosef("unexpected hook content in %s", filename)
				}
			}
			continue
		}

		if !os.IsNotExist(err) {
			dief("checking hook: %v", err)
		}
		verbosef("installing %s hook", hookFile)
		if err := ioutil.WriteFile(filename, []byte(hookContent), 0700); err != nil {
			dief("writing hook: %v", err)
		}
	}
}

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		dief("could not get current directory: %v", err)
	}
	rootlen := 1
	if runtime.GOOS == "windows" {
		rootlen += len(filepath.VolumeName(dir))
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if len(dir) == rootlen && dir[rootlen-1] == filepath.Separator {
			dief("git root not found. Rerun from within the Git tree.")
		}
		dir = filepath.Dir(dir)
	}
}

var hookScript = `#!/bin/sh
exec git-review hook-invoke %s "$@"
`

func hookInvoke(args []string) {
	if len(args) == 0 {
		dief("usage: git-review hook-invoke <hook-name> [args...]")
	}
	switch args[0] {
	case "commit-msg":
		hookCommitMsg(args[1:])
	}
}

// hookCommitMsg is installed as the git commit-msg hook.
// It adds a Change-Id line to the bottom of the commit message
// if there is not one already.
func hookCommitMsg(args []string) {
	// Add Change-Id to commit message if needed.
	if len(args) != 1 {
		dief("usage: git-review hook-invoke commit-msg message.txt\n")
	}
	file := args[0]
	data, err := ioutil.ReadFile(file)
	if err != nil {
		dief("%v", err)
	}
	if bytes.Contains(data, []byte("\nChange-Id: ")) {
		return
	}
	n := len(data)
	for n > 0 && data[n-1] == '\n' {
		n--
	}
	var id [20]byte
	if _, err := io.ReadFull(rand.Reader, id[:]); err != nil {
		dief("generating Change-Id: %v", err)
	}
	data = append(data[:n], fmt.Sprintf("\n\nChange-Id: I%x\n", id[:])...)
	if err := ioutil.WriteFile(file, data, 0666); err != nil {
		dief("%v", err)
	}
}

// This is NOT USED ANYMORE.
// It is here only for comparing against old commit-hook files.
var oldCommitMsgHook = `#!/bin/sh
# From Gerrit Code Review 2.2.1
#
# Part of Gerrit Code Review (http://code.google.com/p/gerrit/)
#
# Copyright (C) 2009 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

CHANGE_ID_AFTER="Bug|Issue"
MSG="$1"

# Check for, and add if missing, a unique Change-Id
#
add_ChangeId() {
	clean_message=` + "`" + `sed -e '
		/^diff --git a\/.*/{
			s///
			q
		}
		/^Signed-off-by:/d
		/^#/d
	' "$MSG" | git stripspace` + "`" + `
	if test -z "$clean_message"
	then
		return
	fi

	if grep -i '^Change-Id:' "$MSG" >/dev/null
	then
		return
	fi

	id=` + "`" + `_gen_ChangeId` + "`" + `
	perl -e '
		$MSG = shift;
		$id = shift;
		$CHANGE_ID_AFTER = shift;

		undef $/;
		open(I, $MSG); $_ = <I>; close I;
		s|^diff --git a/.*||ms;
		s|^#.*$||mg;
		exit unless $_;

		@message = split /\n/;
		$haveFooter = 0;
		$startFooter = @message;
		for($line = @message - 1; $line >= 0; $line--) {
			$_ = $message[$line];

			if (/^[a-zA-Z0-9-]+:/ && !m,^[a-z0-9-]+://,) {
				$haveFooter++;
				next;
			}
			next if /^[ []/;
			$startFooter = $line if ($haveFooter && /^\r?$/);
			last;
		}

		@footer = @message[$startFooter+1..@message];
		@message = @message[0..$startFooter];
		push(@footer, "") unless @footer;

		for ($line = 0; $line < @footer; $line++) {
			$_ = $footer[$line];
			next if /^($CHANGE_ID_AFTER):/i;
			last;
		}
		splice(@footer, $line, 0, "Change-Id: I$id");

		$_ = join("\n", @message, @footer);
		open(O, ">$MSG"); print O; close O;
	' "$MSG" "$id" "$CHANGE_ID_AFTER"
}
_gen_ChangeIdInput() {
	echo "tree ` + "`" + `git write-tree` + "`" + `"
	if parent=` + "`" + `git rev-parse HEAD^0 2>/dev/null` + "`" + `
	then
		echo "parent $parent"
	fi
	echo "author ` + "`" + `git var GIT_AUTHOR_IDENT` + "`" + `"
	echo "committer ` + "`" + `git var GIT_COMMITTER_IDENT` + "`" + `"
	echo
	printf '%s' "$clean_message"
}
_gen_ChangeId() {
	_gen_ChangeIdInput |
	git hash-object -t commit --stdin
}


add_ChangeId
`

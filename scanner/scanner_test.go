/*
 * Copyright (c) 2022 The XGo Authors (xgo.dev). All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package scanner

import (
	"testing"

	"github.com/goplus/xgo/token"
)

func scanComments(t *testing.T, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s Scanner
	s.Init(file, []byte(src), func(pos token.Position, msg string) {
		t.Fatalf("scan error at %v: %s", pos, msg)
	}, ScanComments)
	var comments []string
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			comments = append(comments, lit)
		}
	}
	return comments
}

func TestHashCommentStopsAtLineEnd(t *testing.T) {
	// A '#' comment ends at the end of the line, like a '//' comment. It used
	// to consume one extra character before looking for the end of the line,
	// so a line holding nothing but '#' swallowed the line that followed it.
	got := scanComments(t, "#\n# next\n")
	want := []string{"#", "# next"}
	if len(got) != len(want) {
		t.Fatalf("comments = %q, want %q", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("comment[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestHashCommentText(t *testing.T) {
	got := scanComments(t, "# doc\n#\nx := 1 # trailing\n")
	want := []string{"# doc", "#", "# trailing"}
	if len(got) != len(want) {
		t.Fatalf("comments = %q, want %q", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("comment[%d] = %q, want %q", i, got[i], w)
		}
	}
}

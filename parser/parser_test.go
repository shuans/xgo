/*
 * Copyright (c) 2021 The XGo Authors (xgo.dev). All rights reserved.
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

package parser

import (
	"io/fs"
	"testing"

	"github.com/goplus/xgo/ast"
	"github.com/goplus/xgo/token"
	fsx "github.com/qiniu/x/http/fs"
)

// -----------------------------------------------------------------------------

type fileInfo struct {
	*fsx.FileInfo
}

func (p fileInfo) Info() (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func TestFilter(t *testing.T) {
	d := fsx.NewFileInfo("foo.go", 10)
	if filter(d, func(fi fs.FileInfo) bool {
		return false
	}) {
		t.Fatal("TestFilter: true?")
	}
	if !filter(d, func(fi fs.FileInfo) bool {
		return true
	}) {
		t.Fatal("TestFilter: false?")
	}
	d2 := fileInfo{d}
	if filter(d2, func(fi fs.FileInfo) bool {
		return true
	}) {
		t.Fatal("TestFilter: true?")
	}
}

func TestAssert(t *testing.T) {
	defer func() {
		if e := recover(); e != "go/parser internal error: panic msg" {
			t.Fatal("TestAssert:", e)
		}
	}()
	assert(false, "panic msg")
}

func panicMsg(e any) string {
	switch v := e.(type) {
	case string:
		return v
	case error:
		return v.Error()
	}
	return ""
}

func testErrCode(t *testing.T, code string, errExp, panicExp string) {
	defer func() {
		if e := recover(); e != nil {
			if panicMsg(e) != panicExp {
				t.Fatal("testErrCode panic:", e)
			}
		}
	}()
	t.Helper()
	fset := token.NewFileSet()
	_, err := Parse(fset, "/foo/bar.xgo", code, 0)
	if err == nil || err.Error() != errExp {
		t.Fatal("testErrCode error:", err)
	}
}

func testShadowEntry(t *testing.T, code string, errExp string, decl *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := ParseEntry(fset, "/foo/bar.xgo", code, Config{
		Mode: ParseXGoClass | ParseComments | AllErrors,
	})
	if err != nil && err.Error() != errExp {
		t.Fatal("testShadowEntry error:", err)
	}
	if err == nil && errExp != "" {
		t.Fatal("testShadowEntry: nil, errExp:", errExp)
	}

	if f.ShadowEntry == nil {
		t.Fatal("testShadowEntry: nil")
	}
	if f.ShadowEntry.Body.Lbrace != decl.Body.Lbrace || f.ShadowEntry.Body.Rbrace != decl.Body.Rbrace {
		t.Fatal("testShadowEntry: brace mismatch", f.ShadowEntry.Body.Lbrace, decl.Body.Lbrace, f.ShadowEntry.Body.Rbrace, decl.Body.Rbrace)
	}
}

func testErrCodeParseExpr(t *testing.T, code string, errExp, panicExp string) {
	defer func() {
		if e := recover(); e != nil {
			if panicMsg(e) != panicExp {
				t.Fatal("testErrCodeParseExpr panic:", e)
			}
		}
	}()
	t.Helper()
	_, err := ParseExpr(code)
	if err == nil || err.Error() != errExp {
		t.Fatal("testErrCodeParseExpr error:", err)
	}
}

func testClassErrCode(t *testing.T, code string, errExp, panicExp string) {
	defer func() {
		if e := recover(); e != nil {
			if panicMsg(e) != panicExp {
				t.Fatal("testErrCode panic:", e)
			}
		}
	}()
	t.Helper()
	fset := token.NewFileSet()
	_, err := Parse(fset, "/foo/bar.gox", code, ParseXGoClass)
	if err == nil || err.Error() != errExp {
		t.Fatal("testErrCode error:", err)
	}
}

func TestErrLabel(t *testing.T) {
	testErrCode(t, `a.x:`, `/foo/bar.xgo:1:4: illegal label declaration`, ``)
}

func TestErrTplLit(t *testing.T) {
	testErrCode(t, "tpl`a =`", `/foo/bar.xgo:1:8: expected ';', found 'EOF' (and 1 more errors)`, ``)
}

func TestErrOperand(t *testing.T) {
	testErrCode(t, `a :=`, `/foo/bar.xgo:1:5: expected operand, found 'EOF'`, ``)
}

func TestErrMissingComma(t *testing.T) {
	testErrCode(t, `func a(b int c)`, `/foo/bar.xgo:1:14: missing ',' in parameter list`, ``)
}

func TestErrLambda(t *testing.T) {
	testErrCode(t, `func test(v string, f func( int)) {
}
test "hello" => {
	println "lambda",x
}
`, `/foo/bar.xgo:3:6: expected 'IDENT', found "hello"`, ``)
	testErrCode(t, `func test(v string, f func( int)) {
}
test "hello", "x" => {
	println "lambda",x
}
`, `/foo/bar.xgo:3:15: expected 'IDENT', found "x"`, ``)
	testErrCode(t, `func test(v string, f func( int)) {
}
test "hello", ("x") => {
	println "lambda",x
}
`, `/foo/bar.xgo:3:16: expected 'IDENT', found "x"`, ``)
	testErrCode(t, `func test(v string, f func(int,int)) {
}
test "hello", (x, "y") => {
	println "lambda",x,y
}
`, `/foo/bar.xgo:3:19: expected 'IDENT', found "y"`, ``)
	testErrCode(t, `onTouchStart "someone" => {
	say "touched by someone"
}
`, `/foo/bar.xgo:1:14: expected 'IDENT', found "someone"`, ``)
}

func TestErrTooManyParseExpr(t *testing.T) {
	testErrCodeParseExpr(t, `func() int {
  var
  var
  var
  var
  var
  var
  var
  var
  var
  var
  var
  var
}()
`, `3:3: expected 'IDENT', found 'var' (and 10 more errors)`, ``)
}

func TestErrTooMany(t *testing.T) {
	testErrCode(t, `
func f() { var }
func g() { var }
func h() { var }
func h() { var }
func h() { var }
func h() { var }
func h() { var }
func h() { var }
func h() { var }
func h() { var }
func h() { var }
func h() { var }
`, `/foo/bar.xgo:2:16: expected 'IDENT', found '}' (and 10 more errors)`, ``)
}

func TestErrInFunc(t *testing.T) {
	testErrCode(t, `func test() {
	a,
}`, `/foo/bar.xgo:2:2: expected 1 expression (and 2 more errors)`, ``)
	testErrCode(t, `func test() {
	a.test, => {
	}
}`, `/foo/bar.xgo:2:10: expected operand, found '=>' (and 1 more errors)`, ``)
	testErrCode(t, `func test() {
		,
	}
}`, `/foo/bar.xgo:2:3: expected statement, found ',' (and 1 more errors)`, ``)
}

// -----------------------------------------------------------------------------

func TestClassErrCode(t *testing.T) {
	testClassErrCode(t, `var (
	A,B
	v int
)
`, `/foo/bar.gox:2:2: missing variable type or initialization`, ``)
	testClassErrCode(t, `var (
	A.*B
	v int
)
`, `/foo/bar.gox:2:4: expected 'IDENT', found '*'`, ``)
	testClassErrCode(t, `var (
	[]A
	v int
)
`, `/foo/bar.gox:2:2: expected 'IDENT', found '['`, ``)
	testClassErrCode(t, `var (
	*[]A
	v int
)
`, `/foo/bar.gox:2:3: expected 'IDENT', found '['`, ``)
	testClassErrCode(t, `
var (
)
const c = 100
const d
`, `/foo/bar.gox:5:7: missing constant value`, ``)
}

func TestErrStaticMember(t *testing.T) {
	testClassErrCode(t, `var (
	. B int
)
`, `/foo/bar.gox:2:4: whitespace is not allowed in static member name`, ``)
	testErrCode(t, `var (
	.B int
)
`, `/foo/bar.xgo:2:2: expected 'IDENT', found '.' (and 1 more errors)`, ``)
	testErrCode(t, `var (
	A. B int
)
`, `/foo/bar.xgo:2:5: whitespace is not allowed in static member name`, ``)
	testErrCode(t, `var (
	A .B int
)
`, `/foo/bar.xgo:2:4: whitespace is not allowed in static member name`, ``)
}

func TestErrGlobal(t *testing.T) {
	testErrCode(t, `func test() {}
}`, `/foo/bar.xgo:2:1: expected statement, found '}'`, ``)
}

func TestErrCompositeLiteral(t *testing.T) {
	testErrCode(t, `println (T[int]){a: 1, b: 2}
`, `/foo/bar.xgo:1:10: cannot parenthesize type in composite literal`, ``)
}

func TestErrCondExpr(t *testing.T) {
	testErrCode(t, `
x@*p
`, `/foo/bar.xgo:2:3: expected condition expression, found '*'`, ``)
	testErrCode(t, `
x@(a, b)
`, `/foo/bar.xgo:2:3: invalid condition expression`, ``)
}

func TestErrSelectorExpr(t *testing.T) {
	testErrCode(t, `
x.
*p
`, `/foo/bar.xgo:3:1: expected selector or type assertion, found '*'`, ``)
	testErrCode(t, `
x.$
a = 1
`, "/foo/bar.xgo:3:1: expected identifier after '$', found a", ``)
	testErrCode(t, `
x./
`, `/foo/bar.xgo:2:3: expected selector or type assertion, found '/'`, ``)
	testErrCode(t, `
x.**.$
`, `/foo/bar.xgo:2:6: expected identifier after '**.', found '$'`, ``)
}

func TestErrStringLitEx(t *testing.T) {
	testErrCode(t, `
println "${ ... }"
`, "/foo/bar.xgo:2:13: expected operand, found '...'", ``)
	testErrCode(t, `
println "${b"
`, "/foo/bar.xgo:2:11: invalid $ expression: ${ doesn't end with }", ``)
	testErrCode(t, `
println "$a${b}"
`, "/foo/bar.xgo:2:10: invalid $ expression: neither `${ ... }` nor `$$`", ``)
}

func TestErrStringLiteral(t *testing.T) {
	testErrCode(t, `run "
`, `/foo/bar.xgo:1:5: string literal not terminated`, ``)
}

func TestErrFieldDecl(t *testing.T) {
	testErrCode(t, `
type T struct {
	*(Foo)
}
`, `/foo/bar.xgo:3:3: cannot parenthesize embedded type`, ``)
	testErrCode(t, `
type T struct {
	(Foo)
}
`, `/foo/bar.xgo:3:2: cannot parenthesize embedded type`, ``)
	testErrCode(t, `
type T struct {
	(*Foo)
}
`, `/foo/bar.xgo:3:2: cannot parenthesize embedded type`, ``)
}

func TestParseFieldDecl(t *testing.T) {
	var p parser
	p.init(token.NewFileSet(), "/foo/bar.xgo", []byte(`type T struct {
}
`), 0, nil)
	p.parseFieldDecl(nil)
}

func TestCommentHashStyle(t *testing.T) {
	// '#'-style line comments are equivalent to '//'-style ones, so they must
	// be grouped and attached to declarations in exactly the same way. Before
	// this was fixed, the first line of a '#'-comment block was classified as
	// a line comment (and thus dropped) and only the last line survived as the
	// doc comment.
	fset := token.NewFileSet()
	f, err := ParseFile(fset, "/foo/bar.xgo", `# doc line 1
# doc line 2
func g() {
}
`, ParseComments)
	if err != nil {
		t.Fatal("ParseFile failed:", err)
	}
	decl := f.Decls[0].(*ast.FuncDecl)
	if decl.Doc == nil {
		t.Fatal("g has no doc comment")
	}
	want := []string{"# doc line 1", "# doc line 2"}
	if len(decl.Doc.List) != len(want) {
		t.Fatalf("g doc = %v, want %v", docTexts(decl.Doc), want)
	}
	for i, w := range want {
		if got := decl.Doc.List[i].Text; got != w {
			t.Fatalf("g doc[%d] = %q, want %q", i, got, w)
		}
	}
}

func TestCommentHashStyleBetweenDecls(t *testing.T) {
	// A '#' comment right after a declaration used to be swallowed as a line
	// comment of the preceding token, so it never showed up as the doc comment
	// of the following declaration.
	fset := token.NewFileSet()
	f, err := ParseFile(fset, "/foo/bar.xgo", `func f() {
}

# doc of g
func g() {
}

# doc of h
type h int
`, ParseComments)
	if err != nil {
		t.Fatal("ParseFile failed:", err)
	}
	want := []string{"# doc of g", "# doc of h"}
	var got []string
	for _, d := range f.Decls {
		switch v := d.(type) {
		case *ast.FuncDecl:
			if v.Name.Name == "g" {
				got = append(got, docTexts(v.Doc)...)
			}
		case *ast.GenDecl:
			for _, spec := range v.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == "h" {
					got = append(got, docTexts(v.Doc)...)
				}
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("docs = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("docs[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func docTexts(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	ret := make([]string, len(doc.List))
	for i, c := range doc.List {
		ret[i] = c.Text
	}
	return ret
}

func TestDefaultClassInfo(t *testing.T) {
	_, isProj, ok := DefaultClassInfo("foo.gsh")
	if !isProj || !ok {
		t.Fatal("DefaultClassInfo for foo.gsh failed")
	}
}

func TestCheckExpr(t *testing.T) {
	var p parser
	p.init(token.NewFileSet(), "/foo/bar.xgo", []byte(``), 0, nil)
	p.checkExpr(&ast.Ellipsis{})
	p.checkExpr(&ast.ElemEllipsis{})
	p.checkExpr(&ast.MatrixLit{})
	p.checkExpr(&ast.StarExpr{})
	p.checkExpr(&ast.IndexListExpr{})
	p.checkExpr(&ast.FuncType{})
	p.checkExpr(&ast.FuncLit{})
}

func TestErrType(t *testing.T) {
	testErrCode(t, `var a *
`, `/foo/bar.xgo:1:9: expected type, found 'EOF'`, ``)
}

func TestErrTupleType(t *testing.T) {
	testErrCode(t, `var a (a float64, x int, chan int)
`, `/foo/bar.xgo:1:8: mixed named and unnamed fields in tuple type`, ``)
	testErrCode(t, `var a (a float64, x int, string)
`, `/foo/bar.xgo:1:8: mixed named and unnamed fields in tuple type`, ``)
}

func TestErrFuncDecorator(t *testing.T) {
	testErrCode(t, `@foo
`, `/foo/bar.xgo:1:6: expected 'func', found 'EOF'`, ``)
}

func TestErrFuncDecl(t *testing.T) {
	testErrCode(t, `func test()
{
}
`, `/foo/bar.xgo:2:1: unexpected semicolon or newline before {`, ``)
	testErrCode(t, `func test() +1
`, `/foo/bar.xgo:1:13: expected ';', found '+'`, ``)
	testErrCode(t, `
func (a T) +{}
`, `/foo/bar.xgo:2:12: expected type, found '+'`, ``)
	testErrCode(t, `func +(a T, b T) {}
`, `/foo/bar.xgo:1:6: overload operator can only have one parameter`, ``)
}

func TestErrForIn(t *testing.T) {
	testErrCode(t, `x := [a for a i b]
`, `/foo/bar.xgo:1:15: expected 'in', found i`, ``)
}

func TestErrKwargExpr(t *testing.T) {
	testErrCode(t, `
f a=1, 13
`, `/foo/bar.xgo:2:8: positional argument follows keyword argument`, ``)
}

func TestNumberUnitLit(t *testing.T) {
	var p parser
	p.checkExpr(&ast.NumberUnitLit{})
	p.toIdent(&ast.NumberUnitLit{})
}

func TestImplicitIdent(t *testing.T) {
	if ast.NewIdentEx(100, "foo", ast.ImplicitPkg).End() != 100 {
		t.Fatal("TestImplicitPkg: not 100")
	}
	if ast.NewIdentEx(100, "foo", ast.ImplicitFun).End() != 100 {
		t.Fatal("TestImplicitFun: not 100")
	}
	if ast.NewIdentEx(100, "foo", ast.Fun).End() != 103 {
		t.Fatal("TestFun: not 103")
	}
}

func TestErrGlobalVarWithSyntaxError(t *testing.T) {
	// Parse the code
	testShadowEntry(t, `var (
	foo int.=2
	sprites []Sprite
)

func reset() {
	foo = 10
	sprites = make([]Sprite, 0)
}

onStart => {
	reset()
}
`, `/foo/bar.xgo:2:10: expected 'IDENT', found '=' (and 19 more errors)`, &ast.FuncDecl{
		Body: &ast.BlockStmt{
			Lbrace: 119,
			Rbrace: 121,
		},
	})

	testShadowEntry(t, `var (
	foo int
	sprites []Sprite
)

func reset() {
	foo = 10
	sprites = make([]Sprite, 0)
}

onStart => {
	reset()
}`, "", &ast.FuncDecl{
		Body: &ast.BlockStmt{
			Lbrace: 94,
			Rbrace: 117,
		},
	})
}

// -----------------------------------------------------------------------------

func TestTypeParamDecls(t *testing.T) {
	// Type parameters may be declared on functions and on types, and the
	// parser has to tell a type parameter list apart from an array length.
	fset := token.NewFileSet()
	f, err := ParseFile(fset, "/foo/bar.xgo", `func Sum[T Num](xs []T) T {
	return xs[0]
}

func Map[K comparable, V any](m map[K]V) []V {
	return nil
}

type Pair[T, U any] struct {
	First  T
	Second U
}

type Arr [2]int
type Matrix [2][3]float64
`, ParseComments)
	if err != nil {
		t.Fatal("ParseFile failed:", err)
	}
	sum := f.Decls[0].(*ast.FuncDecl)
	if tp := sum.Type.TypeParams; tp == nil || len(tp.List) != 1 {
		t.Fatalf("Sum type params = %v, want 1 field", tp)
	} else if got := tp.List[0].Names[0].Name; got != "T" {
		t.Fatalf("Sum type param name = %q, want %q", got, "T")
	}
	m := f.Decls[1].(*ast.FuncDecl)
	if tp := m.Type.TypeParams; tp == nil || len(tp.List) != 2 {
		t.Fatalf("Map type params = %v, want 2 fields", tp)
	} else if got, want := tp.List[1].Names[0].Name, "V"; got != want {
		t.Fatalf("Map second type param name = %q, want %q", got, want)
	}
	pair := f.Decls[2].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
	// T and U share the constraint `any`, so they are one field with two names.
	if tp := pair.TypeParams; tp == nil || len(tp.List) != 1 || len(tp.List[0].Names) != 2 {
		t.Fatalf("Pair type params = %v, want 1 field with 2 names", tp)
	} else if got := tp.List[0].Names[0].Name; got != "T" {
		t.Fatalf("Pair first type param name = %q, want %q", got, "T")
	}
	arr := f.Decls[3].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
	if arr.TypeParams != nil {
		t.Fatalf("Arr type params = %v, want nil", arr.TypeParams)
	}
	if _, ok := arr.Type.(*ast.ArrayType); !ok {
		t.Fatalf("Arr type = %T, want *ast.ArrayType", arr.Type)
	}
	matrix := f.Decls[4].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
	if matrix.TypeParams != nil {
		t.Fatalf("Matrix type params = %v, want nil", matrix.TypeParams)
	}
	if _, ok := matrix.Type.(*ast.ArrayType); !ok {
		t.Fatalf("Matrix type = %T, want *ast.ArrayType", matrix.Type)
	}
}

func TestInterfaceTypeSet(t *testing.T) {
	// An interface may embed type sets: a term prefixed with ~, and a union
	// of terms. Both may be mixed with methods and embedded interfaces.
	fset := token.NewFileSet()
	f, err := ParseFile(fset, "/foo/bar.xgo", `type Num interface {
	~int | ~float64
	comparable
	Foo(T) T
	[]byte | string
	map[string]int
}
`, ParseComments)
	if err != nil {
		t.Fatal("ParseFile failed:", err)
	}
	spec := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
	intf, ok := spec.Type.(*ast.InterfaceType)
	if !ok {
		t.Fatalf("Num type = %T, want *ast.InterfaceType", spec.Type)
	}
	list := intf.Methods.List
	if len(list) != 5 {
		t.Fatalf("Num has %d elements, want 5", len(list))
	}
	// ~int | ~float64
	if len(list[0].Names) != 0 {
		t.Fatalf("element 0 has names %v, want none", list[0].Names)
	}
	bin, ok := list[0].Type.(*ast.BinaryExpr)
	if !ok || bin.Op != token.OR {
		t.Fatalf("element 0 type = %v, want a '|' expression", list[0].Type)
	}
	if u, ok := bin.X.(*ast.UnaryExpr); !ok || u.Op != token.TILDE {
		t.Fatalf("element 0 left = %v, want a '~' expression", bin.X)
	}
	// comparable
	if len(list[1].Names) != 0 {
		t.Fatalf("element 1 has names %v, want none", list[1].Names)
	}
	if id, ok := list[1].Type.(*ast.Ident); !ok || id.Name != "comparable" {
		t.Fatalf("element 1 type = %v, want comparable", list[1].Type)
	}
	// Foo(T) T
	if len(list[2].Names) != 1 || list[2].Names[0].Name != "Foo" {
		t.Fatalf("element 2 names = %v, want Foo", list[2].Names)
	}
	if _, ok := list[2].Type.(*ast.FuncType); !ok {
		t.Fatalf("element 2 type = %T, want *ast.FuncType", list[2].Type)
	}
	// []byte | string
	if bin, ok := list[3].Type.(*ast.BinaryExpr); !ok || bin.Op != token.OR {
		t.Fatalf("element 3 type = %v, want a '|' expression", list[3].Type)
	}
	// map[string]int
	if _, ok := list[4].Type.(*ast.MapType); !ok {
		t.Fatalf("element 4 type = %T, want *ast.MapType", list[4].Type)
	}
}

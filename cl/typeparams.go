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

package cl

import (
	"go/types"

	"github.com/goplus/gogen"
	"github.com/goplus/xgo/ast"
	"github.com/goplus/xgo/token"
)

func toTermList(ctx *blockCtx, expr ast.Expr) []*types.Term {
retry:
	switch v := expr.(type) {
	case *ast.UnaryExpr:
		if v.Op != token.TILDE {
			panic(ctx.newCodeErrorf(v.Pos(), v.End(), "invalid op %v must ~", v.Op))
		}
		return []*types.Term{types.NewTerm(true, toType(ctx, v.X))}
	case *ast.BinaryExpr:
		if v.Op != token.OR {
			panic(ctx.newCodeErrorf(v.Pos(), v.End(), "invalid op %v must |", v.Op))
		}
		return append(toTermList(ctx, v.X), toTermList(ctx, v.Y)...)
	case *ast.ParenExpr:
		expr = v.X
		goto retry
	}
	return []*types.Term{types.NewTerm(false, toType(ctx, expr))}
}

func toBinaryExprType(ctx *blockCtx, v *ast.BinaryExpr) types.Type {
	return types.NewInterfaceType(nil, []types.Type{types.NewUnion(toTermList(ctx, v))})
}

func toUnaryExprType(ctx *blockCtx, v *ast.UnaryExpr) types.Type {
	return types.NewInterfaceType(nil, []types.Type{types.NewUnion(toTermList(ctx, v))})
}

func toTypeParams(ctx *blockCtx, params *ast.FieldList) []*types.TypeParam {
	if params == nil {
		return nil
	}
	return collectTypeParams(ctx, params)
}

func toFuncType(ctx *blockCtx, typ *ast.FuncType, recv *types.Var, d *ast.FuncDecl) *types.Signature {
	var typeParams []*types.TypeParam
	var lookup *typeParamLookup
	if recv != nil && d != nil && d.Recv != nil {
		astRecv := d.Recv.List[0].Type
		recv, typeParams = toMethodRecv(ctx, recv, astRecv)
		if len(typeParams) > 0 {
			lookup = methodTypeParamLookup(astRecv, typeParams)
		}
	} else {
		typeParams = toTypeParams(ctx, typ.TypeParams)
		if len(typeParams) > 0 {
			lookup = &typeParamLookup{typeParams: typeParams}
		}
	}
	if lookup != nil {
		ctx.tlookup = lookup
		defer func() {
			ctx.tlookup = nil
		}()
	}
	params, variadic := toParams(ctx, typ.Params.List)
	results := toResults(ctx, typ.Results)
	if recv != nil {
		return types.NewSignatureType(recv, typeParams, nil, params, results, variadic)
	}
	return types.NewSignatureType(recv, nil, typeParams, params, results, variadic)
}

// methodTypeParamLookup returns the lookup that resolves the type parameter
// names used inside a method. The receiver may spell them differently from the
// type declaration, so the names of the receiver are resolved as well.
func methodTypeParamLookup(astRecv ast.Expr, tparams []*types.TypeParam) *typeParamLookup {
	return &typeParamLookup{
		typeParams: tparams,
		aliases:    recvTypeParamAliases(astRecv, tparams),
	}
}

// toMethodRecv returns a method's receiver together with the type parameters of
// the method.
//
// For a method of a generic type the receiver is the base type instantiated
// with those type parameters. That is what go/types does, and it is what makes
// the receiver usable: `p.v` then has the type of the type parameter, the same
// type as a parameter declared as `T`, and a call on an instantiated type is
// substituted correctly because the signature carries the type parameters that
// the instantiation replaces.
func toMethodRecv(ctx *blockCtx, recv *types.Var, astRecv ast.Expr) (*types.Var, []*types.TypeParam) {
	t := recv.Type()
	ptr, _ := t.(*types.Pointer)
	if ptr != nil {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return recv, nil
	}
	typeParams := recvTypeParams(ctx, astRecv, named)
	if len(typeParams) == 0 {
		return recv, nil
	}
	args := make([]types.Type, len(typeParams))
	for i, tp := range typeParams {
		args[i] = tp
	}
	inst := ctx.pkg.Instantiate(named, args, astRecv)
	if ptr != nil {
		inst = types.NewPointer(inst)
	}
	return types.NewVar(recv.Pos(), recv.Pkg(), recv.Name(), inst), typeParams
}

// recvTypeParams returns the type parameters declared by the receiver of a
// method. They are fresh objects, exactly as in go/types, where a method
// declares its own type parameters and only borrows the bounds from the
// receiver's base type. They carry the names of the type declaration, which is
// what the receiver is emitted with; a receiver that spells them differently is
// handled by recvTypeParamAliases.
//
// The type's own type parameters cannot be reused here: they are already bound
// to the type, and binding them a second time panics.
func recvTypeParams(ctx *blockCtx, typ ast.Expr, named *types.Named) []*types.TypeParam {
	orgTypeParams := named.TypeParams()
	if orgTypeParams == nil {
		return nil
	}
	base, indices := recvBase(typ)
	if indices == nil {
		panic(ctx.newCodeErrorf(base.Pos(), base.End(), "cannot use generic type %v without instantiation", named))
	}
	if len(indices) != orgTypeParams.Len() {
		if _, ok := base.(*ast.IndexExpr); ok {
			panic(ctx.newCodeErrorf(base.Pos(), base.End(), "got 1 type parameter, but receiver base type declares %v", orgTypeParams.Len()))
		}
		panic(ctx.newCodeErrorf(base.Pos(), base.End(), "got %v arguments but %v type parameters", len(indices), orgTypeParams.Len()))
	}
	tparams := make([]*types.TypeParam, len(indices))
	for i := range indices {
		tp := orgTypeParams.At(i)
		obj := types.NewTypeName(tp.Obj().Pos(), tp.Obj().Pkg(), tp.Obj().Name(), nil)
		tparams[i] = types.NewTypeParam(obj, types.Typ[types.Invalid])
	}
	// Re-resolve the constraints of the type with the new type parameters in
	// scope. A constraint may refer to a sibling type parameter, e.g.
	// `type Slice[S sliceOf[T], T any]`, and it has to refer to the type
	// parameter of the method for the receiver to be a valid instantiation.
	if spec := typeSpecOf(ctx, named); spec != nil && spec.TypeParams != nil {
		setTypeParamConstraints(ctx, spec.TypeParams, tparams)
	}
	return tparams
}

// typeSpecOf returns the declaration of a type of the package being compiled.
func typeSpecOf(ctx *blockCtx, named *types.Named) *ast.TypeSpec {
	if sym, ok := ctx.syms[named.Obj().Name()]; ok {
		if ld, ok := sym.(*typeLoader); ok {
			return ld.spec
		}
	}
	return nil
}

// recvBase returns a receiver type with its parentheses and pointer stripped,
// together with the type arguments it is written with, e.g. the `E` of
// `*Data[E]`. The type arguments are nil if the receiver has none.
func recvBase(typ ast.Expr) (ast.Expr, []ast.Expr) {
L:
	for {
		switch t := typ.(type) {
		case *ast.ParenExpr:
			typ = t.X
		case *ast.StarExpr:
			typ = t.X
		default:
			break L
		}
	}
	switch t := typ.(type) {
	case *ast.IndexExpr:
		return typ, []ast.Expr{t.Index}
	case *ast.IndexListExpr:
		return typ, t.Indices
	}
	return typ, nil
}

// recvTypeParamAliases returns the names that a receiver gives to the type
// parameters of its method. A receiver may rename them, as in
// `func (p *Data[E]) Get() E` for a type declared as `Data[T]`, and the method
// then refers to them by the names of the receiver. Those names are mapped to
// the type parameters of the signature, which carry the names of the type
// declaration. It returns nil if no name differs.
func recvTypeParamAliases(typ ast.Expr, tparams []*types.TypeParam) map[string]*types.TypeParam {
	_, indices := recvBase(typ)
	if len(indices) != len(tparams) {
		return nil
	}
	var aliases map[string]*types.TypeParam
	for i, index := range indices {
		id, ok := index.(*ast.Ident)
		if !ok {
			continue
		}
		if tp := tparams[i]; id.Name != tp.Obj().Name() {
			if aliases == nil {
				aliases = make(map[string]*types.TypeParam)
			}
			aliases[id.Name] = tp
		}
	}
	return aliases
}

// newFunc creates the gogen function of a declaration, which is a method when
// sig has a receiver.
//
// A method of a generic type needs special care. Its receiver is an
// instantiation, but gogen registers the method on the type of the receiver and
// types.Named.AddMethod refuses a type that has type arguments. The method is
// therefore registered here on the base generic type first, and gogen is handed
// a receiver without type arguments so that its own registration becomes a
// no-op. The function object is then replaced by the registered one, so that
// the body sees the instantiated receiver.
func newFunc(ctx *blockCtx, pos token.Pos, name string, sig *types.Signature, recvTypePos func() token.Pos) (*gogen.Func, error) {
	if recv := sig.Recv(); recv != nil && name != "_" {
		if base, ok := genericBase(recv.Type()); ok {
			fn := types.NewFunc(pos, ctx.pkg.Types, name, sig)
			base.AddMethod(fn)
			safe, err := ctx.pkg.NewFuncWith(pos, name, genericRecvSig(recv, sig), recvTypePos)
			if err != nil {
				return nil, err
			}
			safe.Func = fn
			return safe, nil
		}
	}
	return ctx.pkg.NewFuncWith(pos, name, sig, recvTypePos)
}

// genericBase returns the generic type that typ was instantiated from. It
// reports false if typ is not an instantiated named type.
func genericBase(typ types.Type) (*types.Named, bool) {
	if ptr, _ := typ.(*types.Pointer); ptr != nil {
		typ = ptr.Elem()
	}
	named, ok := typ.(*types.Named)
	if !ok || named.TypeArgs() == nil {
		return nil, false
	}
	return named.Origin(), true
}

// genericRecvSig returns sig with its receiver replaced by the generic type the
// receiver was instantiated from.
func genericRecvSig(recv *types.Var, sig *types.Signature) *types.Signature {
	typ := recv.Type()
	if ptr, _ := typ.(*types.Pointer); ptr != nil {
		typ = types.NewPointer(ptr.Elem().(*types.Named).Origin())
	} else {
		typ = typ.(*types.Named).Origin()
	}
	base := types.NewVar(recv.Pos(), recv.Pkg(), recv.Name(), typ)
	return types.NewSignatureType(base, nil, nil, sig.Params(), sig.Results(), sig.Variadic())
}

// setTypeParamLookup makes the type parameters of sig visible through
// ctx.tlookup, so that the body of a generic function or method can refer to
// them. The type parameters of a function live in sig.TypeParams(); those of a
// method come from its receiver (sig.RecvTypeParams()), and the receiver may
// rename them. src is the declaration the body belongs to. It reports whether
// ctx.tlookup was changed.
func setTypeParamLookup(ctx *blockCtx, sig *types.Signature, src ast.Node) bool {
	tparams := sig.RecvTypeParams()
	recvParams := tparams != nil && tparams.Len() > 0
	if !recvParams {
		tparams = sig.TypeParams()
	}
	if tparams == nil || tparams.Len() == 0 {
		return false
	}
	list := make([]*types.TypeParam, tparams.Len())
	for i := range list {
		list[i] = tparams.At(i)
	}
	var lookup *typeParamLookup
	if recvParams {
		if d, ok := src.(*ast.FuncDecl); ok && d.Recv != nil && len(d.Recv.List) > 0 {
			lookup = methodTypeParamLookup(d.Recv.List[0].Type, list)
		}
	}
	if lookup == nil {
		lookup = &typeParamLookup{typeParams: list}
	}
	ctx.tlookup = lookup
	return true
}

type typeParamLookup struct {
	typeParams []*types.TypeParam
	aliases    map[string]*types.TypeParam
}

func (p *typeParamLookup) Lookup(name string) *types.TypeParam {
	if t, ok := p.aliases[name]; ok {
		return t
	}
	for _, t := range p.typeParams {
		tname := t.Obj().Name()
		if tname != "_" && name == tname {
			return t
		}
	}
	return nil
}

func initType(ctx *blockCtx, named *types.Named, spec *ast.TypeSpec) {
	typeParams := toTypeParams(ctx, spec.TypeParams)
	if len(typeParams) > 0 {
		named.SetTypeParams(typeParams)
		ctx.tlookup = &typeParamLookup{typeParams: typeParams}
		defer func() {
			ctx.tlookup = nil
		}()
	}
	org := ctx.inInst
	ctx.inInst = 0
	defer func() {
		ctx.inInst = org
	}()
	typ := toType(ctx, spec.Type)
retry:
	switch t := typ.(type) {
	case *types.Named:
		typ = getUnderlying(ctx, t)
	case *types.Alias:
		typ = types.Unalias(t)
		goto retry
	}
	named.SetUnderlying(typ)
}

func getRecvType(expr ast.Expr) (typ ast.Expr, ptr bool, ok bool) {
	typ = expr
L:
	for {
		switch t := typ.(type) {
		case *ast.ParenExpr:
			typ = t.X
		case *ast.StarExpr:
			if ptr {
				ok = false
				return
			}
			ptr = true
			typ = t.X
		default:
			break L
		}
	}
	switch t := typ.(type) {
	case *ast.IndexExpr:
		typ = t.X
	case *ast.IndexListExpr:
		typ = t.X
	}
	ok = true
	return
}

func collectTypeParams(ctx *blockCtx, list *ast.FieldList) []*types.TypeParam {
	var tparams []*types.TypeParam
	// Declare type parameters up-front, with empty interface as type bound.
	// The scope of type parameters starts at the beginning of the type parameter
	// list (so we can have mutually recursive parameterized interfaces).
	for _, f := range list.List {
		tparams = declareTypeParams(ctx, tparams, f.Names)
	}
	setTypeParamConstraints(ctx, list, tparams)
	return tparams
}

// setTypeParamConstraints sets the constraint of each type parameter of list.
// The constraints are resolved with all of tparams in scope, so that they may
// refer to each other.
func setTypeParamConstraints(ctx *blockCtx, list *ast.FieldList, tparams []*types.TypeParam) {
	ctx.tlookup = &typeParamLookup{typeParams: tparams}
	defer func() {
		ctx.tlookup = nil
	}()

	index := 0
	for _, f := range list.List {
		var bound types.Type
		// NOTE: we may be able to assert that f.Type != nil here, but this is not
		// an invariant of the AST, so we are cautious.
		if f.Type != nil {
			bound = boundTypeParam(ctx, f.Type)
			if isTypeParam(bound) {
				// We may be able to allow this since it is now well-defined what
				// the underlying type and thus type set of a type parameter is.
				// But we may need some additional form of cycle detection within
				// type parameter lists.
				//check.error(f.Type, MisplacedTypeParam, "cannot use a type parameter as constraint")
				bound = types.Typ[types.Invalid]
			} else if t, ok := bound.(*types.Named); ok {
				if t.Underlying() == nil { // check named underlying is nil
					ctx.loadNamed(ctx.pkg, t)
				}
			}
		} else {
			bound = types.Typ[types.Invalid]
		}
		for i := range f.Names {
			tparams[index+i].SetConstraint(bound)
		}
		index += len(f.Names)
	}
}

func declareTypeParams(ctx *blockCtx, tparams []*types.TypeParam, names []*ast.Ident) []*types.TypeParam {
	// Use Typ[Invalid] for the type constraint to ensure that a type
	// is present even if the actual constraint has not been assigned
	// yet.
	// TODO(gri) Need to systematically review all uses of type parameter
	//           constraints to make sure we don't rely on them if they
	//           are not properly set yet.
	for _, name := range names {
		tname := types.NewTypeName(name.Pos(), ctx.pkg.Types, name.Name, nil)
		tpar := types.NewTypeParam(tname, types.Typ[types.Invalid]) // assigns type to tpar as a side-effect
		// check.declare(check.scope, name, tname, check.scope.pos)    // TODO(gri) check scope position
		tparams = append(tparams, tpar)
	}

	return tparams
}

func isTypeParam(t types.Type) bool {
	_, ok := t.(*types.TypeParam)
	return ok
}

func isSpecificSliceType(ctx *blockCtx, typ types.Type) bool {
	if typ == nil {
		return false
	}
	var t *types.Slice
	switch tt := typ.(type) {
	case *types.Named:
		t = getUnderlying(ctx, tt).(*types.Slice)
	case *types.Slice:
		t = tt
	default:
		return false
	}
	_, ok := t.Elem().(*types.TypeParam)
	return !ok
}

func boundTypeParam(ctx *blockCtx, x ast.Expr) types.Type {
	// A type set literal of the form ~T and A|B may only appear as constraint;
	// embed it in an implicit interface so that only interface type-checking
	// needs to take care of such type expressions.
	wrap := false
	switch op := x.(type) {
	case *ast.UnaryExpr:
		wrap = op.Op == token.TILDE
	case *ast.BinaryExpr:
		wrap = op.Op == token.OR
	}
	if wrap {
		x = &ast.InterfaceType{Methods: &ast.FieldList{List: []*ast.Field{{Type: x}}}}
		t := toType(ctx, x)
		// mark t as implicit interface if all went well
		if t, _ := t.(*types.Interface); t != nil {
			t.MarkImplicit()
		}
		return t
	}
	return toType(ctx, x)
}

func namedIsTypeParams(ctx *blockCtx, t *types.Named) bool {
	o := t.Obj()
	if o.Pkg() == ctx.pkg.Types {
		if _, ok := ctx.generics[o.Name()]; !ok {
			return false
		}
		ctx.loadType(o.Name())
	}
	return t.Obj() != nil && t.TypeArgs() == nil && t.TypeParams() != nil
}

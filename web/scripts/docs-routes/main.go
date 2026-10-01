// Documentation tooling: inspect Go syntax without executing the gateway or reading configuration.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type group struct {
	path       string
	middleware []string
}
type environment map[string]any
type extractor struct {
	files     *token.FileSet
	functions map[string]*ast.FuncDecl
	globals   map[string]ast.Expr
}

func (x *extractor) source(node ast.Node) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, x.files, node)
	return strings.ReplaceAll(strings.ReplaceAll(b.String(), "\n", " "), "\t", " ")
}

func (x *extractor) value(expr ast.Expr, env environment) any {
	switch e := expr.(type) {
	case *ast.BasicLit:
		s, _ := strconv.Unquote(e.Value)
		return s
	case *ast.Ident:
		if v, ok := env[e.Name]; ok {
			return v
		}
		if v, ok := x.globals[e.Name]; ok {
			return x.value(v, env)
		}
	case *ast.SelectorExpr:
		if owner, ok := e.X.(*ast.Ident); ok && owner.Name == "http" {
			return strings.ToUpper(strings.TrimPrefix(e.Sel.Name, "Method"))
		}
		if m, ok := x.value(e.X, env).(environment); ok {
			return m[e.Sel.Name]
		}
		return x.source(e)
	case *ast.CompositeLit:
		fields := environment{}
		list := []any{}
		for _, element := range e.Elts {
			if kv, ok := element.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					fields[key.Name] = x.value(kv.Value, env)
				}
			} else {
				list = append(list, x.value(element, env))
			}
		}
		if len(fields) > 0 {
			return fields
		}
		return list
	}
	return nil
}

func (x *extractor) render(node ast.Expr, env environment) string {
	result := x.source(node)
	for name, value := range env {
		if fields, ok := value.(environment); ok {
			for field, item := range fields {
				if text, ok := item.(string); ok {
					result = strings.ReplaceAll(result, name+"."+field, text)
				}
			}
		}
	}
	return result
}

func (x *extractor) emit(method, route string, middleware []string, handler string, node ast.Node) {
	if strings.Contains(handler, "RelayNotImplemented") || route == "" || method == "" {
		return
	}
	pos := x.files.Position(node.Pos())
	fmt.Printf("%s\t%s\t%s\t%s\t%s:%d\n", method, route, strings.Join(middleware, " | "), handler, pos.Filename, pos.Line)
}

func (x *extractor) call(call *ast.CallExpr, env environment) *group {
	if name, ok := call.Fun.(*ast.Ident); ok {
		fn := x.functions[name.Name]
		if fn == nil || fn.Body == nil {
			return nil
		}
		next := environment{}
		i := 0
		found := false
		for _, field := range fn.Type.Params.List {
			for _, parameter := range field.Names {
				if i < len(call.Args) {
					next[parameter.Name] = x.value(call.Args[i], env)
					if _, ok := next[parameter.Name].(*group); ok {
						found = true
					}
				}
				i++
			}
		}
		if found {
			x.block(fn.Body, next)
		}
		return nil
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	parent, ok := x.value(selector.X, env).(*group)
	if !ok {
		return nil
	}
	if selector.Sel.Name == "Group" && len(call.Args) > 0 {
		part, ok := x.value(call.Args[0], env).(string)
		if !ok {
			return nil
		}
		joined := path.Join(parent.path, part)
		if !strings.HasPrefix(joined, "/") {
			joined = "/" + joined
		}
		if strings.HasSuffix(part, "/") && joined != "/" {
			joined += "/"
		}
		return &group{joined, slices.Clone(parent.middleware)}
	}
	if selector.Sel.Name == "Use" {
		for _, arg := range call.Args {
			parent.middleware = append(parent.middleware, x.render(arg, env))
		}
		return nil
	}
	method := selector.Sel.Name
	start := 1
	routeArg := 0
	if method == "Handle" {
		if len(call.Args) < 2 {
			return nil
		}
		method, _ = x.value(call.Args[0], env).(string)
		routeArg = 1
		start = 2
	}
	if !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}, method) || len(call.Args) <= routeArg {
		return nil
	}
	part, ok := x.value(call.Args[routeArg], env).(string)
	if !ok {
		return nil
	}
	joined := path.Join(parent.path, part)
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	if strings.HasSuffix(part, "/") && joined != "/" {
		joined += "/"
	}
	middleware := slices.Clone(parent.middleware)
	handlers := []string{}
	for _, arg := range call.Args[start:] {
		s := x.render(arg, env)
		handlers = append(handlers, s)
		if strings.HasPrefix(s, "middleware.") {
			middleware = append(middleware, s)
		}
	}
	x.emit(method, joined, middleware, strings.Join(handlers, " | "), call)
	return nil
}

func (x *extractor) block(block *ast.BlockStmt, env environment) {
	for _, statement := range block.List {
		switch s := statement.(type) {
		case *ast.AssignStmt:
			for i, right := range s.Rhs {
				if i >= len(s.Lhs) {
					break
				}
				name, ok := s.Lhs[i].(*ast.Ident)
				if !ok {
					continue
				}
				if call, ok := right.(*ast.CallExpr); ok {
					if v := x.call(call, env); v != nil {
						env[name.Name] = v
					}
				} else {
					env[name.Name] = x.value(right, env)
				}
			}
		case *ast.ExprStmt:
			if c, ok := s.X.(*ast.CallExpr); ok {
				x.call(c, env)
			}
		case *ast.BlockStmt:
			next := environment{}
			for k, v := range env {
				next[k] = v
			}
			x.block(s, next)
		case *ast.RangeStmt:
			list, ok := x.value(s.X, env).([]any)
			if !ok {
				continue
			}
			name, ok := s.Value.(*ast.Ident)
			if !ok {
				continue
			}
			for _, v := range list {
				next := environment{}
				for k, item := range env {
					next[k] = item
				}
				next[name.Name] = v
				x.block(s.Body, next)
			}
		}
	}
}

func main() {
	x := extractor{token.NewFileSet(), map[string]*ast.FuncDecl{}, map[string]ast.Expr{}}
	names, err := filepath.Glob("../router/*.go")
	if err != nil {
		panic(err)
	}
	names = append(names, "../pkg/jsplugin/routing.go")
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(x.files, name, nil, 0)
		if err != nil {
			panic(err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				x.functions[d.Name.Name] = d
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if v, ok := s.(*ast.ValueSpec); ok {
						for i, n := range v.Names {
							if i < len(v.Values) {
								x.globals[n.Name] = v.Values[i]
							}
						}
					}
				}
			}
		}
	}
	for _, name := range []string{"SetApiRouter", "SetDashboardRouter", "SetRelayRouter", "SetTaskRouter", "SetVideoRouter"} {
		fn := x.functions[name]
		if fn == nil {
			fmt.Fprintln(os.Stderr, "Missing router", name)
			os.Exit(1)
		}
		x.block(fn.Body, environment{"router": &group{}})
	}
	// Host-owned protocol URLs are data, not handwritten aliases in the documentation.
	protocols, _ := x.value(x.globals["hostProtocols"], environment{}).([]any)
	for _, p := range protocols {
		protocol := p.(environment)
		operations, _ := protocol["Operations"].([]any)
		for _, o := range operations {
			operation := o.(environment)
			methods, _ := operation["Methods"].([]any)
			key := protocol["Name"].(string) + "." + operation["Name"].(string)
			var handlers []string
			var location ast.Node
			ast.Inspect(x.functions["taskPluginProtocolHandlers"].Body, func(node ast.Node) bool {
				clause, ok := node.(*ast.CaseClause)
				if !ok {
					return true
				}
				for _, value := range clause.List {
					if x.value(value, environment{}) != key {
						continue
					}
					for _, stmt := range clause.Body {
						ret, ok := stmt.(*ast.ReturnStmt)
						if !ok || len(ret.Results) == 0 {
							continue
						}
						list, ok := ret.Results[0].(*ast.CompositeLit)
						if !ok {
							continue
						}
						location = ret
						for _, handler := range list.Elts {
							handlers = append(handlers, x.source(handler))
						}
					}
				}
				return false
			})
			if location == nil {
				panic("Host protocol has no documented handler: " + key)
			}
			var middleware []string
			for _, handler := range handlers {
				if strings.HasPrefix(handler, "middleware.") {
					middleware = append(middleware, handler)
				}
			}
			for _, m := range methods {
				x.emit(m.(string), operation["Path"].(string), middleware, strings.Join(handlers, "; "), location)
			}
		}
	}
}

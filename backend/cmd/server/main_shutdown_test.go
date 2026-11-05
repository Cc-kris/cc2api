package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 关停超时后仍要执行 defer app.Cleanup()，把缓冲中的用量和计费写完再退出（V01-R15 / V01-AC48）。
// log.Fatalf 会直接 os.Exit(1)，跳过所有 defer，因此这里静态校验 runMainServer 的源码结构：
// 1) defer app.Cleanup() 出现在 Shutdown 之前；2) Shutdown 的错误分支不得使用 log.Fatal*。
func TestRunMainServer_ShutdownTimeoutDoesNotSkipCleanup(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, parser.ParseComments)
	require.NoError(t, err)

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "runMainServer" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn, "runMainServer 未找到")

	var (
		cleanupDeferPos  token.Pos
		shutdownCallPos  token.Pos
		fatalInShutdown  bool
		shutdownLogCalls []string
	)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeferStmt:
			if exprString(node.Call.Fun) == "app.Cleanup" && !cleanupDeferPos.IsValid() {
				cleanupDeferPos = node.Pos()
			}
		case *ast.IfStmt:
			// if err := app.Server.Shutdown(ctx); err != nil { ... }
			assign, ok := node.Init.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 {
				return true
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok || !strings.HasSuffix(exprString(call.Fun), "Server.Shutdown") {
				return true
			}
			shutdownCallPos = node.Pos()
			ast.Inspect(node.Body, func(inner ast.Node) bool {
				innerCall, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := exprString(innerCall.Fun)
				if strings.HasPrefix(name, "log.") {
					shutdownLogCalls = append(shutdownLogCalls, name)
				}
				if strings.HasPrefix(name, "log.Fatal") || name == "os.Exit" {
					fatalInShutdown = true
				}
				return true
			})
		}
		return true
	})

	require.True(t, cleanupDeferPos.IsValid(), "runMainServer 里没有 defer app.Cleanup()")
	require.True(t, shutdownCallPos.IsValid(), "runMainServer 里没有 app.Server.Shutdown 调用")
	require.Less(t, int(cleanupDeferPos), int(shutdownCallPos),
		"defer app.Cleanup() 必须注册在 Shutdown 之前，否则关停时不会执行")
	require.False(t, fatalInShutdown,
		"Shutdown 失败分支不能用 log.Fatal*/os.Exit：会跳过 defer app.Cleanup()，缓冲中的用量会丢")
	require.Contains(t, shutdownLogCalls, "log.Printf",
		"Shutdown 失败分支应用 log.Printf 打印 \"Server forced to shutdown\"")
}

func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		return exprString(e.Fun)
	}
	return ""
}

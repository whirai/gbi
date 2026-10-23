package logger_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 覆盖豁免表(与 specs/2026-10-11-method-call-logging-design.md 一致;追加须注明理由)。
var coverageExemptPkgs = map[string]string{
	"logger": "日志设施自身,打日志会递归",
}
var coverageExemptFuncs = map[string]string{
	"service.coerceValue": "每单元格调用(20万行导入≈400万次),唯一每行爆炸点",
	"datacore.isEmptyRow": "每数据行调用(5000行导入≈+1万行,20万行≈+40万行),逐行爆炸点",
}

// coveragePkgs 相对 internal/ 的包目录名。
var coveragePkgs = []string{"service", "api", "agent", "datacore", "llm", "logger", "webapp", "db", "rbac", "auth", "model", "config", "assets"}

// funcName 生成 "包.[类型.]函数名" 全名(与豁免表键一致)。
func funcName(pkg string, fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		t := fn.Recv.List[0].Type
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X
		}
		if id, ok := t.(*ast.Ident); ok {
			return pkg + "." + id.Name + "." + fn.Name.Name
		}
	}
	return pkg + "." + fn.Name.Name
}

// hasLogMarker 检查函数体 AST 中是否存在首参字符串含 mark 的 slog.<Level> 调用。
func hasLogMarker(fn *ast.FuncDecl, mark string) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != "slog" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if strings.Contains(lit.Value, mark) {
			found = true
		}
		return true
	})
	return found
}

// TestMethodCoverage 全库方法覆盖检查:非豁免函数/方法必须含「方法进入」与「方法离开」日志。
// 开发期用 COVERAGE_PKG=<pkg> 只对本包报错;收口(最终验收)不带过滤,要求全绿。
func TestMethodCoverage(t *testing.T) {
	filter := os.Getenv("COVERAGE_PKG")
	missingByPkg := map[string][]string{}
	covered, exempt, missing := 0, 0, 0

	scan := func(pkgDir, pkgName string) {
		files, err := filepath.Glob(filepath.Join(pkgDir, "*.go"))
		require.NoError(t, err)
		// 空 Glob 会让覆盖检查静默"假绿"(如包目录改名/路径错误),必须硬失败。
		require.NotEmpty(t, files, "包目录 %s 无 .go 文件(目录改名或路径错误?)", pkgDir)
		fset := token.NewFileSet()
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, file, nil, 0)
			require.NoError(t, err)
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				name := funcName(pkgName, fn)
				if _, ok := coverageExemptPkgs[pkgName]; ok {
					exempt++
					continue
				}
				if _, ok := coverageExemptFuncs[name]; ok {
					exempt++
					continue
				}
				if hasLogMarker(fn, "方法进入") && hasLogMarker(fn, "方法离开") {
					covered++
					continue
				}
				missing++
				missingByPkg[pkgName] = append(missingByPkg[pkgName], name)
			}
		}
	}

	for _, p := range coveragePkgs {
		scan(filepath.Join("..", p), p)
	}
	scan(filepath.Join("..", "..", "cmd", "gbi"), "gbi")

	for _, p := range coveragePkgs {
		if filter != "" && p != filter {
			continue
		}
		if bucket := missingByPkg[p]; len(bucket) > 0 {
			t.Errorf("[%s] 缺漏 %d 个: %s", p, len(bucket), strings.Join(bucket, ", "))
		}
	}
	if filter == "" {
		if bucket := missingByPkg["gbi"]; len(bucket) > 0 {
			t.Errorf("[gbi] 缺漏 %d 个: %s", len(bucket), strings.Join(bucket, ", "))
		}
	}
	t.Logf("覆盖=%d 豁免=%d 缺漏=%d", covered, exempt, missing)
}

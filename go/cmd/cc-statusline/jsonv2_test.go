package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// テストコードも含め、JSON 処理を v1 に戻さない。
func TestJSONUsesV2(t *testing.T) {
	t.Parallel()
	err := filepath.WalkDir(filepath.Join(repoRoot(t), "go"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if name == "encoding/json" {
				t.Errorf("%s imports JSON v1; use encoding/json/v2", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

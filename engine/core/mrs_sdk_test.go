package mywant

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPySDKIsWrittenOut(t *testing.T) {
	dir := pySDKPath()
	if dir == "" {
		t.Fatal("not written")
	}
	if _, err := os.Stat(filepath.Join(dir, "mywant", "plugin.py")); err != nil {
		t.Fatal(err)
	}
}

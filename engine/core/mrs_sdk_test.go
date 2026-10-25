package mywant

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The embedded SDK is a copy of client-py/mywant; `make pysdk` brings it up
// to date.
func TestPySDKMatchesClientPy(t *testing.T) {
	src := filepath.Join("..", "..", "client-py", "mywant")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Skip("client-py not beside the engine")
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".py" {
			continue
		}
		want, _ := os.ReadFile(filepath.Join(src, e.Name()))
		got, err := pySDK.ReadFile("pysdk/mywant/" + e.Name())
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("engine/core/pysdk/mywant/%s differs from client-py — run `make pysdk`", e.Name())
		}
	}
}

func TestPySDKIsWrittenOut(t *testing.T) {
	dir := pySDKPath()
	if dir == "" {
		t.Fatal("not written")
	}
	if _, err := os.Stat(filepath.Join(dir, "mywant", "plugin.py")); err != nil {
		t.Fatal(err)
	}
}

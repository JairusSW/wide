package wide

import (
	"os"
	"testing"

	wago "github.com/wago-org/wago"
)

func TestEmittedAssemblyScriptFixture(t *testing.T) {
	path := os.Getenv("AS_SIMD_EMITTED_FIXTURE")
	if path == "" {
		t.Skip("set AS_SIMD_EMITTED_FIXTURE for transform integration")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := wago.NewRuntime().Compile(b)
	if err != nil {
		t.Fatalf("compile without plugin: %v", err)
	}
	if plain.Compiled().RequiresAVX2() {
		t.Fatal("plugin-free compile unexpectedly selected wide code")
	}
	rt := wago.NewRuntime()
	if err := rt.Use(New()); err != nil {
		t.Fatal(err)
	}
	native, err := rt.Compile(b)
	if err != nil {
		t.Fatalf("plugin compile: %v", err)
	}
	if !native.Compiled().RequiresAVX2() {
		t.Fatal("custom instruction imports did not select native wide code")
	}
}

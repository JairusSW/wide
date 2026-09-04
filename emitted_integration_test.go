//go:build !tinygo

package wide

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	wago "github.com/wago-org/wago"
)

// TestAssemblyScriptTransformParity compiles as-simd's wide fixture both with
// and without the Wide transform, then executes both modules. It is opt-in so
// normal Go-only consumers do not need Node or an as-simd checkout.
func TestAssemblyScriptTransformParity(t *testing.T) {
	asSIMD := os.Getenv("AS_SIMD_DIR")
	var portableWasm, nativeWasm []byte
	if asSIMD == "" {
		portablePath, nativePath := os.Getenv("AS_SIMD_PORTABLE_FIXTURE"), os.Getenv("AS_SIMD_WIDE_FIXTURE")
		if portablePath == "" || nativePath == "" {
			t.Skip("set AS_SIMD_DIR or both AS_SIMD_PORTABLE_FIXTURE and AS_SIMD_WIDE_FIXTURE")
		}
		var err error
		portableWasm, err = os.ReadFile(portablePath)
		if err != nil {
			t.Fatal(err)
		}
		nativeWasm, err = os.ReadFile(nativePath)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		var err error
		asSIMD, err = filepath.Abs(asSIMD)
		if err != nil {
			t.Fatal(err)
		}
		node, err := exec.LookPath("node")
		if err != nil {
			t.Fatal("AS_SIMD_DIR is set but node is unavailable")
		}
		asc := filepath.Join(asSIMD, "node_modules", "assemblyscript", "bin", "asc.js")
		if _, err := os.Stat(asc); err != nil {
			t.Fatalf("AssemblyScript compiler unavailable at %s; install as-simd dependencies: %v", asc, err)
		}

		output := t.TempDir()
		compile := func(name string, useWide bool) []byte {
			t.Helper()
			path := filepath.Join(output, name+".wasm")
			args := []string{
				asc,
				"bench/wide/bench.ts",
				"--runtime", "stub",
				"--transform", "./transform",
				"-O3", "--converge",
				"--enable", "simd",
				"-o", path,
			}
			cmd := exec.Command(node, args...)
			cmd.Dir = asSIMD
			cmd.Env = withoutEnv(os.Environ(), "WAGO_PLUGINS")
			if useWide {
				cmd.Env = append(cmd.Env, "WAGO_PLUGINS=wide")
			}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("compile as-simd %s fixture: %v\n%s", name, err, out)
			}
			wasm, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			return wasm
		}
		portableWasm = compile("portable", false)
		nativeWasm = compile("wide", true)
	}

	portableRuntime := wago.NewRuntime()
	defer portableRuntime.Close()
	portableModule, err := portableRuntime.Compile(portableWasm)
	if err != nil {
		t.Fatalf("compile portable fixture: %v", err)
	}
	portable, err := portableRuntime.Instantiate(context.Background(), portableModule)
	if err != nil {
		t.Fatalf("instantiate portable fixture: %v", err)
	}
	defer portable.Close()

	plainRuntime := wago.NewRuntime()
	defer plainRuntime.Close()
	plainModule, err := plainRuntime.Compile(nativeWasm)
	if err != nil {
		t.Fatalf("compile Wide fixture without plugin: %v", err)
	}
	if instance, err := plainRuntime.Instantiate(context.Background(), plainModule); err == nil {
		_ = instance.Close()
		t.Fatal("Wide fixture instantiated without its plugin")
	}

	wideRuntime := wago.NewRuntime()
	defer wideRuntime.Close()
	if err := loadWide(wideRuntime, Config{}); err != nil {
		t.Fatal(err)
	}
	wideModule, err := wideRuntime.Compile(nativeWasm)
	if err != nil {
		t.Fatalf("compile transformed fixture with Wide: %v", err)
	}
	requiresX86Wide := wideModule.Compiled().RequiresAVX2() || wideModule.Compiled().RequiresAVX512()
	if requiresX86Wide != (runtime.GOARCH == "amd64") {
		t.Fatalf("x86 wide requirement=%v on %s", requiresX86Wide, runtime.GOARCH)
	}
	native, err := wideRuntime.Instantiate(context.Background(), wideModule)
	if err != nil {
		t.Fatalf("instantiate transformed fixture with Wide: %v", err)
	}
	defer native.Close()

	iterations := wago.I32(3)
	a := wago.I64(0x102030405060708)
	b := wago.I64(0x1122334455667788)
	tests := []struct {
		name string
		args []uint64
	}{
		{"v256AddI8", []uint64{iterations, a, b}},
		{"v256MulI64", []uint64{iterations, a, b}},
		{"v256MinI32", []uint64{iterations, a, b}},
		{"v256NegI16", []uint64{iterations, a}},
		{"v256AddSatI16", []uint64{iterations, a, b}},
		{"v256Bitselect", []uint64{iterations, a, b}},
		{"v256Load", []uint64{iterations, a}},
		{"v256Store", []uint64{iterations, a}},
		{"v512AddI8", []uint64{iterations, a, b}},
		{"v512MulI64", []uint64{iterations, a, b}},
		{"v512MinI32", []uint64{iterations, a, b}},
		{"v512NegI64", []uint64{iterations, a}},
		{"v512AddSatI16", []uint64{iterations, a, b}},
		{"v512Bitselect", []uint64{iterations, a, b}},
		{"v512Load", []uint64{iterations, a}},
		{"v512Store", []uint64{iterations, a}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want, err := portable.Invoke(tc.name, tc.args...)
			if err != nil {
				t.Fatalf("portable: %v", err)
			}
			got, err := native.Invoke(tc.name, tc.args...)
			if err != nil {
				t.Fatalf("Wide: %v", err)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("Wide result %x, portable result %x", got, want)
			}
		})
	}
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := env[:0]
	for _, entry := range env {
		if len(entry) < len(prefix) || entry[:len(prefix)] != prefix {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

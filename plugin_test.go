package wide

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	wago "github.com/wago-org/wago"
)

func TestRegistersCompleteKernelInstructionCatalog(t *testing.T) {
	rt := wago.NewRuntime()
	if err := rt.Use(New()); err != nil {
		t.Fatal(err)
	}
	imports := rt.ProvidedImports()
	want := len(canonicalNames) * 2
	// Three wago:abi lifecycle imports accompany every custom-instruction plugin.
	if got := len(imports) - 3; got != want {
		t.Fatalf("registered instructions=%d, want %d", got, want)
	}
	for _, spec := range imports {
		if spec.Module == "wago:abi" {
			continue
		}
		if spec.Module != InstructionModule {
			t.Fatalf("unexpected module %q", spec.Module)
		}
		if strings.Contains(spec.Name, ".fd.") {
			t.Fatalf("%s exposes an engine opcode instead of a SIMD semantic name", spec.Name)
		}
		if len(spec.Results) != 0 {
			t.Fatalf("%s returns values", spec.Name)
		}
		for _, typ := range spec.Params {
			if typ != wago.ValI32 {
				t.Fatalf("%s has non-i32 parameter", spec.Name)
			}
		}
	}
}

func TestV256AndV512ImportsLowerNativelyAndExecute(t *testing.T) {
	for _, bits := range []uint16{256, 512} {
		t.Run(fmt.Sprintf("v%d", bits), func(t *testing.T) {
			wasm := kernelImportModule(bits, 81, 3) // v128.xor mirrored across the width
			rt := wago.NewRuntime()
			if err := rt.Use(New()); err != nil {
				t.Fatal(err)
			}
			mod, err := rt.Compile(wasm)
			if err != nil {
				t.Fatal(err)
			}
			if got := mod.Compiled().RequiresAVX2(); got != (runtime.GOARCH == "amd64") {
				t.Fatalf("RequiresAVX2=%v on %s", got, runtime.GOARCH)
			}
			in, err := rt.Instantiate(context.Background(), mod)
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
			memory := in.Memory().Bytes()
			n := int(bits / 8)
			for i := 0; i < n; i++ {
				memory[64+i] = byte(i*3 + 1)
				memory[128+i] = byte(255 - i*5)
			}
			if _, err := in.Invoke("run", wago.I32(0), wago.I32(64), wago.I32(128)); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < n; i++ {
				want := byte(i*3+1) ^ byte(255-i*5)
				if memory[i] != want {
					t.Fatalf("byte %d=%#x, want %#x", i, memory[i], want)
				}
			}
		})
	}
}

func hostHasAVX512() bool {
	data, err := os.ReadFile("/proc/cpuinfo")
	return err == nil && strings.Contains(string(data), " avx512f ") &&
		strings.Contains(string(data), " avx512dq ") &&
		strings.Contains(string(data), " avx512bw ")
}

func TestV512FallsBackWhenAVX512Disabled(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64-only selection")
	}
	t.Setenv("WAGO_DISABLE_AVX512", "1")
	rt := wago.NewRuntime()
	if err := rt.Use(New()); err != nil {
		t.Fatal(err)
	}
	mod, err := rt.Compile(kernelImportModule(512, 81, 3))
	if err != nil {
		t.Fatal(err)
	}
	if mod.Compiled().RequiresAVX512() {
		t.Fatal("AVX-512 lowering selected while disabled")
	}
}

func TestV512ZMMMatchesYMMFallback(t *testing.T) {
	if runtime.GOARCH != "amd64" || !hostHasAVX512() {
		t.Skip("AVX-512F/DQ/BW unavailable")
	}
	direct := []uint32{
		78, 80, 81, 82, 96, 110, 111, 112, 113, 114, 115, 118, 119, 120, 121, 123,
		128, 130, 142, 143, 144, 145, 146, 147, 149, 150, 151, 152, 153, 155,
		160, 174, 177, 181, 182, 183, 184, 185, 186, 192, 206, 209, 213,
		227, 228, 229, 230, 231, 234, 235, 239, 240, 241, 242, 243, 246, 247,
		265, 266, 267, 268, 269, 270, 271, 272, 273,
	}
	old, hadOld := os.LookupEnv("WAGO_DISABLE_AVX512")
	oldForce, hadForce := os.LookupEnv("WAGO_FORCE_AVX512")
	defer func() {
		if hadOld {
			_ = os.Setenv("WAGO_DISABLE_AVX512", old)
		} else {
			_ = os.Unsetenv("WAGO_DISABLE_AVX512")
		}
		if hadForce {
			_ = os.Setenv("WAGO_FORCE_AVX512", oldForce)
		} else {
			_ = os.Unsetenv("WAGO_FORCE_AVX512")
		}
	}()
	for _, sub := range direct {
		op, ok := operationFor(sub)
		if !ok {
			t.Fatalf("missing operation %d", sub)
		}
		arity, ok := op.kernelArity()
		if !ok {
			t.Fatalf("operation %d is not a kernel", sub)
		}
		name, _ := instructionName(512, sub)
		t.Run(name, func(t *testing.T) {
			type compiled struct {
				rt  *wago.Runtime
				mod *wago.Module
			}
			compile := func(disable bool) compiled {
				if disable {
					_ = os.Setenv("WAGO_DISABLE_AVX512", "1")
					_ = os.Unsetenv("WAGO_FORCE_AVX512")
				} else {
					_ = os.Unsetenv("WAGO_DISABLE_AVX512")
					_ = os.Setenv("WAGO_FORCE_AVX512", "1")
				}
				rt := wago.NewRuntime()
				if err := rt.Use(New()); err != nil {
					t.Fatal(err)
				}
				mod, err := rt.Compile(kernelImportModule(512, sub, arity+1))
				if err != nil {
					t.Fatal(err)
				}
				return compiled{rt: rt, mod: mod}
			}
			fallback, native := compile(true), compile(false)
			if !native.mod.Compiled().RequiresAVX512() {
				t.Fatal("direct ZMM lowering not selected")
			}
			run := func(c compiled) [64]byte {
				instance, err := c.rt.Instantiate(context.Background(), c.mod)
				if err != nil {
					t.Fatal(err)
				}
				defer instance.Close()
				memory := instance.Memory().Bytes()
				args := make([]uint64, arity+1)
				for input := 0; input < arity; input++ {
					ptr := 64 + input*64
					args[input+1] = wago.I32(int32(ptr))
					for i := 0; i < 64; i++ {
						memory[ptr+i] = byte(i*37 + input*91 + 11)
					}
				}
				if _, err := instance.Invoke("run", args...); err != nil {
					t.Fatal(err)
				}
				var out [64]byte
				copy(out[:], memory[:64])
				return out
			}
			if got, want := run(native), run(fallback); got != want {
				t.Fatalf("ZMM result differs from YMM fallback\n got %x\nwant %x", got, want)
			}
		})
	}
}

func TestInstructionPhysicalSignatureIsChecked(t *testing.T) {
	rt := wago.NewRuntime()
	if err := rt.Use(New()); err != nil {
		t.Fatal(err)
	}
	// XOR needs dst + two source i32 values; this module declares only two.
	if _, err := rt.Compile(kernelImportModule(256, 81, 2)); err == nil {
		t.Fatal("invalid instruction signature accepted")
	}
}

func TestWideSIMDChecksCompleteRangesBeforeWriting(t *testing.T) {
	for _, bits := range []uint16{256, 512} {
		t.Run(fmt.Sprintf("v%d", bits), func(t *testing.T) {
			rt := wago.NewRuntime()
			if err := rt.Use(New()); err != nil {
				t.Fatal(err)
			}
			mod, err := rt.Compile(kernelImportModule(bits, 81, 3))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := rt.Instantiate(context.Background(), mod)
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close()
			memory := instance.Memory().Bytes()
			for i := 0; i < int(bits/8); i++ {
				memory[i] = 0xa5
			}
			// Only half of the final source vector is in bounds.
			bad := uint32(len(memory) - int(bits/16))
			if _, err := instance.Invoke("run", wago.I32(0), wago.I32(64), wago.I32(int32(bad))); err == nil {
				t.Fatal("out-of-bounds full-width source did not trap")
			}
			for i := 0; i < int(bits/8); i++ {
				if memory[i] != 0xa5 {
					t.Fatalf("destination byte %d changed before range validation", i)
				}
			}
		})
	}
}

func TestConstantRangeProofRejectsPartialVectors(t *testing.T) {
	for _, bits := range []uint16{256, 512} {
		t.Run(fmt.Sprintf("v%d", bits), func(t *testing.T) {
			rt := wago.NewRuntime()
			if err := rt.Use(New()); err != nil {
				t.Fatal(err)
			}
			bad := int32(65536 - int(bits/16))
			mod, err := rt.Compile(kernelLoopModule(bits, 81, bad))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := rt.Instantiate(context.Background(), mod)
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close()
			if _, err := instance.Invoke("run", wago.I32(1)); err == nil {
				t.Fatal("partially in-bounds constant vector did not trap")
			}
		})
	}
}

func TestEverySemanticImportCompilesNatively(t *testing.T) {
	rt := wago.NewRuntime()
	if err := rt.Use(New()); err != nil {
		t.Fatal(err)
	}
	for sub := range canonicalNames {
		op, ok := operationFor(sub)
		if !ok {
			t.Fatalf("semantic opcode %d is missing its shape", sub)
		}
		arity, ok := op.kernelArity()
		if !ok {
			t.Fatalf("semantic opcode %d is not a vector kernel", sub)
		}
		for _, bits := range []uint16{256, 512} {
			name, _ := instructionName(bits, sub)
			t.Run(name, func(t *testing.T) {
				mod, err := rt.Compile(kernelImportModule(bits, sub, arity+1))
				if err != nil {
					t.Fatal(err)
				}
				requiresWide := mod.Compiled().RequiresAVX2() || mod.Compiled().RequiresAVX512()
				if requiresWide != (runtime.GOARCH == "amd64") {
					t.Fatalf("native amd64 SIMD requirement=%v on %s", requiresWide, runtime.GOARCH)
				}
			})
		}
	}
}

func BenchmarkWideSIMDWrapper(b *testing.B) {
	const iterations = int32(10000)
	for _, bits := range []uint16{256, 512} {
		b.Run(fmt.Sprintf("v%d", bits), func(b *testing.B) {
			rt := wago.NewRuntime()
			if err := rt.Use(New()); err != nil {
				b.Fatal(err)
			}
			mod, err := rt.Compile(kernelLoopModule(bits, 81, 128))
			if err != nil {
				b.Fatal(err)
			}
			instance, err := rt.Instantiate(context.Background(), mod)
			if err != nil {
				b.Fatal(err)
			}
			defer instance.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := instance.Invoke("run", wago.I32(iterations)); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(iterations), "ns/wide-op")
		})
	}
}

func BenchmarkV512I64Mul(b *testing.B) {
	const iterations = 10_000
	rt := wago.NewRuntime()
	if err := rt.Use(New()); err != nil {
		b.Fatal(err)
	}
	mod, err := rt.Compile(kernelLoopModule(512, 213, 128))
	if err != nil {
		b.Fatal(err)
	}
	instance, err := rt.Instantiate(context.Background(), mod)
	if err != nil {
		b.Fatal(err)
	}
	defer instance.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := instance.Invoke("run", wago.I32(iterations)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/iterations, "ns/wide-op")
}

func kernelImportModule(bits uint16, sub uint32, arity int) []byte {
	vec := func(items ...[]byte) []byte {
		out := uleb(uint32(len(items)))
		for _, item := range items {
			out = append(out, item...)
		}
		return out
	}
	section := func(id byte, body []byte) []byte {
		return append(append([]byte{id}, uleb(uint32(len(body)))...), body...)
	}
	name := func(s string) []byte { return append(uleb(uint32(len(s))), s...) }
	typeBody := []byte{0x60, byte(arity)}
	for i := 0; i < arity; i++ {
		typeBody = append(typeBody, 0x7f)
	}
	typeBody = append(typeBody, 0)
	instruction, ok := instructionName(bits, sub)
	if !ok {
		panic(fmt.Sprintf("unknown SIMD subopcode %d", sub))
	}
	imp := append(name(InstructionModule), name(instruction)...)
	imp = append(imp, 0, 0)
	body := []byte{0}
	for i := 0; i < arity; i++ {
		body = append(body, 0x20, byte(i))
	}
	body = append(body, 0x10, 0, 0x0b)
	code := append(uleb(uint32(len(body))), body...)
	out := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	out = append(out, section(1, vec(typeBody))...)
	out = append(out, section(2, vec(imp))...)
	out = append(out, section(3, vec([]byte{0}))...)
	out = append(out, section(5, vec([]byte{0, 1}))...)
	out = append(out, section(7, vec(append(name("run"), 0, 1), append(name("memory"), 2, 0)))...)
	out = append(out, section(10, vec(code))...)
	return out
}

func kernelLoopModule(bits uint16, sub uint32, right int32) []byte {
	vec := func(items ...[]byte) []byte {
		out := uleb(uint32(len(items)))
		for _, item := range items {
			out = append(out, item...)
		}
		return out
	}
	section := func(id byte, body []byte) []byte {
		return append(append([]byte{id}, uleb(uint32(len(body)))...), body...)
	}
	name := func(s string) []byte { return append(uleb(uint32(len(s))), s...) }
	instruction, ok := instructionName(bits, sub)
	if !ok {
		panic(fmt.Sprintf("unknown SIMD subopcode %d", sub))
	}
	importType := []byte{0x60, 3, 0x7f, 0x7f, 0x7f, 0}
	runType := []byte{0x60, 1, 0x7f, 0}
	imp := append(name(InstructionModule), name(instruction)...)
	imp = append(imp, 0, 0)
	body := []byte{
		0,          // local declarations
		0x02, 0x40, // block
		0x03, 0x40, // loop
		0x20, 0, 0x45, // local.get 0; i32.eqz
		0x0d, 1, // br_if block
		0x41, 0, // destination = 0
		0x41, 0xc0, 0x00, // left = 64
		0x41, // right
	}
	body = append(body, sleb32(right)...)
	body = append(body,
		0x10, 0, // call import
		0x20, 0, 0x41, 1, // local.get 0; i32.const 1
		0x6b, 0x21, 0, // i32.sub; local.set 0
		0x0c, 0, // br loop
		0x0b, 0x0b, 0x0b, // end loop, block, function
	)
	code := append(uleb(uint32(len(body))), body...)
	out := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	out = append(out, section(1, vec(importType, runType))...)
	out = append(out, section(2, vec(imp))...)
	out = append(out, section(3, vec([]byte{1}))...)
	out = append(out, section(5, vec([]byte{0, 1}))...)
	out = append(out, section(7, vec(append(name("run"), 0, 1)))...)
	out = append(out, section(10, vec(code))...)
	return out
}

func sleb32(value int32) []byte {
	var out []byte
	for {
		b := byte(value & 0x7f)
		value >>= 7
		done := value == 0 && b&0x40 == 0 || value == -1 && b&0x40 != 0
		if !done {
			b |= 0x80
		}
		out = append(out, b)
		if done {
			return out
		}
	}
}

func uleb(value uint32) []byte {
	var out []byte
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if value == 0 {
			return out
		}
	}
}

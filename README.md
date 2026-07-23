# Wide

[![CI](https://github.com/JairusSW/wide/actions/workflows/ci.yml/badge.svg)](https://github.com/JairusSW/wide/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/JairusSW/wide.svg)](https://pkg.go.dev/github.com/JairusSW/wide)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Wide is the portable v256/v512 instruction plugin for
[Wago](https://github.com/wago-org/wago). It supplies architecture-neutral
wide-SIMD declarations for ordinary validated i32 imports and lets Wago select
AVX-512/ZMM, AVX2/YMM, or NEON without exposing machine instructions to Wasm.

The initial producer integration is the AssemblyScript
[`as-simd`](https://github.com/JairusSW/as-simd) transform. The guest import
module remains `"as-simd"` for compatibility with its emitted binaries; that is
an ABI name, not the plugin’s package or product name.

```go
import (
    "github.com/JairusSW/wide"
    wago "github.com/wago-org/wago"
)

rt := wago.NewRuntime()
if err := rt.Use(wide.New()); err != nil { /* handle */ }
mod, err := rt.Compile(wasmBytes)
```

The guest ABI contains only Wasm-SIMD-style semantic names—for example
`i8x32.add`, `i64x8.eq`, and `v512.xor`. It contains no AVX, YMM, NEON, numeric
`0xfd` opcode, encoder, or physical-register concept. The plugin declares each
operation once through Wago's target-independent SIMD descriptor. Wago
validates every physical i32 import signature and privately selects
AVX-512/ZMM or AVX2/YMM on amd64 and NEON on arm64.

The catalog covers every mirrored standard SIMD form used by Wago's constrained
wide lowering: arithmetic, comparisons, shifts, saturation, widening and
narrowing, conversions, reductions, lane operations, shuffle and swizzle,
memory operations, constants, relaxed SIMD, and cross-half operations.

Two semantic families are registered:

- widened 256-bit names such as `i8x32.add` use checked pointer instructions of
  the form `(dst, one-to-three inputs) -> ()`;
- widened 512-bit names such as `i8x64.add` preserve the same pointer contract
  for 64-byte chunk-independent unary, binary, and ternary operations.

The import names do not change between targets. Wago lowers v256 to one YMM
chunk on amd64. For v512 it selects a direct ZMM instruction when the CPU has
AVX-512F/DQ/BW and that instruction is an exact semantic match; otherwise it
uses two YMM chunks. On Zen 4, whose 512-bit operations execute as 256-bit
halves, a cost model keeps simple operations on YMM and reserves ZMM for
instruction-collapsing wins such as bitselect and `i64x8.mul`. Arm64 uses
two/four NEON chunks.

Set `WAGO_DISABLE_AVX512=1` to force the portable AVX2 fallback for testing.
`WAGO_FORCE_AVX512=1` forces every available direct ZMM lowering, which is
useful for differential testing and tuning.

The plugin is required to instantiate modules containing these imports. SIMD
and SWAR builds produced with `AS_SIMD_WIDE_INTRINSICS=0` remain standalone.

Run the plugin tests with:

```bash
go test ./...
```

The test suite checks the complete registered semantic catalog, signature
rejection, backend selection, and byte-exact v256/v512 execution on amd64 and
arm64.

## Wrapper performance

Wago validates each dynamic pointer once for the complete vector width and
reuses its native address across every chunk. Constant ranges proven inside the
module's minimum memory need no runtime bounds check. On a Ryzen 7 7800X3D,
10,000-operation in-Wasm loops measured:

| Width | Before | Optimized | Delta |
| --- | ---: | ---: | ---: |
| v256 | 1.086 ns/op | 0.755 ns/op | 30.5% faster |
| v512 | 1.606 ns/op | 1.134 ns/op | 29.4% faster |

Both paths report zero Go allocations per invocation. Run the benchmark with:

```bash
go test -run '^$' -bench BenchmarkWideSIMDWrapper -benchmem
```

The width-aware selector matters on the Ryzen 7 7800X3D: forcing ZMM for a
simple `v512.xor` measured 1.258 ns/op versus 1.145 ns/op for two YMM chunks,
while direct AVX-512 `i64x8.mul` measured 1.248 ns/op versus 1.551 ns/op for the
AVX2 sequence (19.5% faster). Benchmark the complex case with
`-bench BenchmarkV512I64Mul`.

For end-to-end transform integration:

```bash
AS_SIMD_EMITTED_FIXTURE=/path/to/module.wasm go test -run TestEmittedAssemblyScriptFixture
```

## License

MIT. See [LICENSE](LICENSE).

<div align="center">
    <h1><code>wide</code></h1>
    <p>Portable v256 and v512 SIMD instructions for the <a href="https://github.com/wago-org/wago">Wago</a> WebAssembly runtime, lowered to AVX-512, AVX2, or NEON.</p>
</div>

<p align="center">
    <a href="https://github.com/JairusSW/wide/actions/workflows/ci.yml"><img src="https://github.com/JairusSW/wide/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-%3E%3D1.22-00ADD8.svg" alt="Go >= 1.22"></a>
    <a href="https://github.com/wago-org/wago"><img src="https://img.shields.io/badge/wago-%3E%3D0.1.0-6E56CF.svg" alt="Wago >= 0.1.0"></a>
</p>

<details>
<summary>Table of Contents</summary>

- [Overview](#overview)
- [Installation](#installation)
- [Usage](#usage)
  - [Raw Wasm quick start](#raw-wasm-quick-start)
  - [From AssemblyScript](#from-assemblyscript)
- [Instruction ABI](#instruction-abi)
- [Native backends](#native-backends)
- [Compatibility](#compatibility)
- [Performance](#performance)
- [Testing](#testing)
- [Architecture](#architecture)
- [Contributing](#contributing)
- [License](#license)
- [Contact](#contact)

</details>

## Overview

`wide` is an optional [Wago](https://github.com/wago-org/wago) compiler plugin
that turns architecture-neutral v256 and v512 imports into native wide-SIMD
machine code. Guest modules expose ordinary, validated function imports;
operation results use compiler-erased `externref` carriers so expression chains
stay in registers. Wide selects AVX-512/ZMM, AVX2/YMM, or NEON without exposing physical
registers or target mnemonics to Wasm.

What you get out of the box:

- **One portable instruction ABI**: semantic names such as `i8x32.add`,
  `i64x8.eq`, and `v512.xor` are identical on every host architecture.
- **Full-width native lowering**: v256 uses one YMM operation on amd64; v512
  uses a profitable ZMM operation or two YMM operations. Arm64 uses two or four
  NEON chunks.
- **Checked memory access**: each pointer is validated for the complete vector
  width before any destination bytes are written.
- **Register-resident expressions**: `externref` is erased by Wago during
  compilation; it never allocates a host object or enters the runtime reference
  store.
- **Wasm-SIMD semantics**: the catalog mirrors unary, binary, and ternary
  standard and relaxed SIMD kernels at 256- and 512-bit widths.
- **Language-neutral integration**: any compiler or hand-written Wasm module
  can call Wide with ordinary function imports. No custom section or custom
  Wasm type is required.

> **Stability:** experimental (`v0.2.0`). The plugin ABI and backend selection
> policy may change before `v1.0.0`.

## Installation

If you have the [`wago`](https://github.com/wago-org/wago) CLI installed:

```sh
wago pkg add github.com/JairusSW/wide
```

or use [`go get`](https://pkg.go.dev/cmd/go#hdr-Get_packages_and_dependencies):

```sh
go get github.com/JairusSW/wide
```

Wide requires the privileged `compiler.codegen` capability. `wago pkg add`
scaffolds the dependency with no authority; after reviewing the plugin, grant
the capability in your project's `wago.json`:

```json
{
  "dependencies": ["github.com/JairusSW/wide"],
  "plugins": [
    {
      "name": "github.com/JairusSW/wide",
      "capabilities": ["compiler.codegen"]
    }
  ]
}
```

Programmatic `Runtime.Use` is the trusted embedder path and does not require a
manifest grant.

## Usage

### Raw Wasm quick start

Wide does not require AssemblyScript or `as-simd`. This complete WAT module
adds two vectors of eight i32 lanes:

```wat
(module
  (import "as-simd" "v256.load"
    (func $v256.load (param i32) (result externref)))
  (import "as-simd" "i32x8.add"
    (func $i32x8.add (param externref externref) (result externref)))
  (import "as-simd" "v256.store"
    (func $v256.store (param externref i32)))

  (memory (export "memory") 1)

  ;; Eight little-endian i32 values: [1, 2, 3, 4, 5, 6, 7, 8].
  (data (i32.const 32)
    "\01\00\00\00\02\00\00\00\03\00\00\00\04\00\00\00"
    "\05\00\00\00\06\00\00\00\07\00\00\00\08\00\00\00")

  ;; Eight little-endian i32 values: [10, 20, 30, 40, 50, 60, 70, 80].
  (data (i32.const 64)
    "\0a\00\00\00\14\00\00\00\1e\00\00\00\28\00\00\00"
    "\32\00\00\00\3c\00\00\00\46\00\00\00\50\00\00\00")

  (func (export "run")
    i32.const 32  ;; left vector
    call $v256.load
    i32.const 64  ;; right vector
    call $v256.load
    call $i32x8.add
    i32.const 0   ;; destination
    call $v256.store))
```

These are standard Wasm imports using standard `i32` and `externref` types.
`v256.load` turns a checked linear-memory range into a compiler-erased vector,
`i32x8.add` consumes and returns erased vectors, and `v256.store` writes the
result. Under Wago the `externref` values are native register bundles, not
runtime references, so this sequence performs two vector loads, one native add,
and one vector store. `"as-simd"` is only the historical guest ABI namespace;
this example does not install or run the `as-simd` package or transform.

Compile and run the checked-in example:

```sh
wat2wasm examples/raw/add.wat -o add.wasm
go run ./examples/raw add.wasm
```

Output:

```text
[11 22 33 44 55 66 77 88]
```

The host is ordinary Go:

```go
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"

	"github.com/JairusSW/wide"
	wago "github.com/wago-org/wago"
)

func main() {
	wasmBytes, err := os.ReadFile("add.wasm")
	if err != nil {
		log.Fatal(err)
	}

	rt := wago.NewRuntime()
	if err := rt.Use(wide.New()); err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	mod, err := rt.Compile(wasmBytes)
	if err != nil {
		log.Fatal(err)
	}

	instance, err := rt.Instantiate(context.Background(), mod)
	if err != nil {
		log.Fatal(err)
	}
	defer instance.Close()

	if _, err := instance.Invoke("run"); err != nil {
		log.Fatal(err)
	}

	memory := instance.Memory().Bytes()
	lanes := make([]uint32, 8)
	for i := range lanes {
		lanes[i] = binary.LittleEndian.Uint32(memory[i*4:])
	}
	fmt.Println(lanes)
}
```

Registering the plugin before compilation lets Wago recognize and lower these
imports. Without it, they remain ordinary unresolved Wasm imports. An already
compiled native artifact does not need the plugin to execute.

To use a different operation or width, change the semantic import name and
load/store width. v256 loads and stores 32 bytes; v512 loads and stores 64
bytes. The complete naming pattern is described in
[Instruction ABI](#instruction-abi).

### From AssemblyScript

Install and use [`as-simd`](https://github.com/JairusSW/as-simd):

```sh
npm install as-simd
npx asc assembly/index.ts --transform as-simd --enable simd
```

The transform recognizes eligible adjacent v128 chunks and emits the Wide
imports automatically:

```ts
import { v256, v512 } from "as-simd";

const a = v256.splat<i16>(4);
const b = v256.add<i16>(a, a);

const x = v512.splat<i64>(10);
const y = v512.mul<i64>(x, x);
```

The guest import module remains `"as-simd"` for compatibility with existing
generated binaries. That string is the Wasm ABI namespace; the product, plugin,
and Go package are named `wide`.

Set `AS_SIMD_WIDE_INTRINSICS=0` while compiling AssemblyScript to keep the
portable v128-backed implementation and omit Wide imports.

## Instruction ABI

Every semantic operation is an ordinary Wasm import. The default carrier `C`
is `externref`:

| Shape | Wasm signature |
| --- | --- |
| Load | `(address: i32) -> C` |
| Unary | `(input: C) -> C` |
| Binary | `(left: C, right: C) -> C` |
| Ternary | `(a: C, b: C, c: C) -> C` |
| Store | `(value: C, address: i32) -> ()` |

An embedder may instead select `i32`, `i64`, `f32`, `f64`, `v128`, or
`funcref`. The guest and plugin must use the same carrier:

```go
if err := rt.Use(wide.New(wide.WithCarrier(wago.WasmV128))); err != nil {
	panic(err)
}
```

For AssemblyScript, pair that with `AS_SIMD_WIDE_CARRIER=v128` when invoking
the `as-simd` transform. Selecting a carrier changes only the module's
validation signature. Wago still assigns the registered `wide.v256` or
`wide.v512` identity and rejects ordinary values of the same Wasm type at a
custom-instruction boundary.

Import names scale standard SIMD lane shapes to the selected width:

| Standard shape | v256 import | v512 import |
| --- | --- | --- |
| `v128.xor` | `v256.xor` | `v512.xor` |
| `i8x16.add` | `i8x32.add` | `i8x64.add` |
| `i16x8.mul` | `i16x16.mul` | `i16x32.mul` |
| `i32x4.eq` | `i32x8.eq` | `i32x16.eq` |
| `i64x2.mul` | `i64x4.mul` | `i64x8.mul` |
| `f32x4.add` | `f32x8.add` | `f32x16.add` |
| `f64x2.add` | `f64x4.add` | `f64x8.add` |

Wago validates each imported physical signature before code generation. The
guest never observes AVX opcodes, YMM/ZMM registers, NEON registers, numeric
`0xfd` subopcodes, or encoder details.

Erased custom values have expression lifetime: nest them directly through Wide
imports, then store or drop the result. They cannot be put in Wasm locals,
returned from guest functions, passed to ordinary functions, or carried across
control flow. Wago rejects those escapes at compilation instead of silently
materializing them.

For low-level compatibility, every semantic operation also has a `.memory`
form with the former pointer ABI: unary is `(dst, input)`, binary is
`(dst, left, right)`, and ternary is `(dst, a, b, c)`, all `i32` parameters and
no result. That form is useful as a single-operation boundary but reloads and
stores between chained operations.

## Native backends

| Target | v256 | v512 |
| --- | --- | --- |
| amd64 with AVX-512F/DQ/BW | one YMM operation | cost-selected ZMM or two YMM operations |
| amd64 with AVX2 | one YMM operation | two YMM operations |
| arm64 | two NEON chunks | four NEON chunks |

Wide chooses full-width instructions when they reduce work. On CPUs that
execute 512-bit operations as 256-bit halves, its cost model keeps simple
operations on YMM and reserves ZMM for instruction-collapsing wins such as
bitselect and `i64x8.mul`.

Two environment switches support differential testing:

| Variable | Effect |
| --- | --- |
| `WIDE_DISABLE_AVX512=1` | Force the AVX2/YMM v512 fallback |
| `WIDE_FORCE_AVX512=1` | Force every available direct ZMM lowering |

These switches are diagnostic controls, not guest-visible ABI.

## Compatibility

| Axis | Support |
| --- | --- |
| Wago engine | `>= 0.1.0` |
| Go toolchain | `>= 1.22` |
| TinyGo toolchain | `>= 0.41.1`; build Wago hosts with `-scheduler=tasks` |
| Guest ABI | standard `i32` and `externref` Wasm imports under `"as-simd"`; optional pointer-only `.memory` imports |
| `linux/amd64` | AVX2 required; AVX-512F/DQ/BW selected when available and profitable |
| `linux/arm64` | NEON |
| Producer | `as-simd` transform; any language may emit the same imports |

Identity, engine constraints, platforms, and registry metadata live in
[`wago.json`](wago.json).

## Performance

On a Ryzen 7 7800X3D, 10,000-operation in-Wasm loops report zero Go
allocations per invocation. Comparing the two available v512 lowering
strategies:

- `v512.xor`: 1.145 ns/op with two YMM chunks versus 1.258 ns/op with forced
  ZMM, so Wago selects YMM;
- `i64x8.mul`: 1.248 ns/op with direct AVX-512 versus 1.551 ns/op through the
  AVX2 sequence, so Wago selects ZMM for a 19.5% improvement.

A dependent chain of 128 `not` operations demonstrates the cost removed by the
erased-reference ABI. Median results over five benchmark runs were:

| Width | `.memory` round trip per operation | register-resident `externref` | Speedup |
| --- | ---: | ---: | ---: |
| v256 | 2.568 ns/op | 0.447 ns/op | 5.74x |
| v512 | 2.796 ns/op | 0.512 ns/op | 5.46x |

Both paths report zero Go allocations. The difference is native linear-memory
traffic: the `externref` chain loads once, remains in vector registers for all
128 operations, and stores once.

Run the benchmarks locally:

```sh
go test -run '^$' -bench 'Benchmark(WideSIMDWrapper|V512I64Mul|ExternrefExpressionChain)$' -benchmem
```

## Testing

```sh
go test -race ./...
```

The self-contained suite:

- verifies the complete registered v256/v512 semantic catalog;
- rejects mismatched physical import signatures;
- checks full-width bounds before destination mutation;
- compiles every semantic import through the native backend;
- executes dependent erased-reference chains without intermediate memory;
- byte-compares direct ZMM output with the YMM fallback;
- executes representative v256 and v512 modules end to end.

To test a real module emitted by the AssemblyScript transform:

```sh
AS_SIMD_EMITTED_FIXTURE=/path/to/module.wasm \
  go test -run TestEmittedAssemblyScriptFixture
```

CI runs formatting, vet, race tests with coverage, and an arm64 test-binary
cross-compile.

## Architecture

- **`plugin.go`** - extension identity and registration of the architecture-
  neutral instruction declarations.
- **`catalog.go`** - canonical Wasm-SIMD semantic names, operation shapes, lane
  scaling, and pointer arity.
- **`lowering_x86.go`** - AVX2/YMM and SSE-width semantic lowering.
- **`lowering_zmm.go`** - AVX-512 encodings and Wide's ZMM selection policy.
- **`lowering_neon.go`** - complete arm64 NEON semantic lowering.
- **`lowering.go`** - checked full-vector memory access and target adapters.
- **`plugin_test.go`** - catalog, validation, backend-selection, execution, and
  benchmark coverage.
- **`emitted_integration_test.go`** - optional end-to-end coverage for a real
  `as-simd` transform output.
- **`wago.json`** - package manifest for registry identity, engine
  compatibility, and supported platforms.

Wide owns the semantic import catalog, CPU-feature policy, and all AVX-512,
AVX2, and NEON lowering. Wago owns validation, checked-address construction,
register allocation, and raw target encoders. This keeps machine-code access
out of the guest ABI and makes the same Wasm module portable across supported
hosts without placing Wide-specific semantics in Wago.

At registration, Wide declares `wide.v256` and `wide.v512` through Wago's
custom-type registry. `WasmExternRef` is the default carrier; the registered
identity and native register bundles are compiler-only. Wide and other plugins
may instead select `WasmI32`, `WasmI64`, `WasmF32`, `WasmF64`, `WasmV128`, or
`WasmFuncRef` without changing Wago's instruction-lowering interface.

## Contributing

Contributions are welcome! Please:

- Run `go test -race ./...` and `go vet ./...` before opening a pull request.
- Keep guest-visible names architecture-neutral and Wasm-SIMD-style.
- Add every new semantic operation at both v256 and v512 widths.
- Differential-test direct full-width lowering against its portable chunked
  fallback.
- Follow standard Go formatting (`gofmt`) and conventional commit messages.

## License

This project is distributed under the [MIT License](./LICENSE). Work on this
project is done out of passion - if you want to support it financially, you can
donate through [GitHub Sponsors](https://github.com/sponsors/JairusSW).

## Contact

Please file issues at [GitHub Issues](https://github.com/JairusSW/wide/issues).
To chat, join the [Wago Discord](https://wago.sh/discord).

- **GitHub:** [https://github.com/JairusSW/wide](https://github.com/JairusSW/wide)
- **Website:** [https://wago.sh/](https://wago.sh/)
- **Discord:** [https://wago.sh/discord](https://wago.sh/discord)

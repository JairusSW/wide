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
  - [On a Runtime](#on-a-runtime)
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
machine code. Guest modules expose ordinary, validated i32 function imports;
Wago privately selects AVX-512/ZMM, AVX2/YMM, or NEON without exposing physical
registers or target mnemonics to Wasm.

What you get out of the box:

- **One portable instruction ABI**: semantic names such as `i8x32.add`,
  `i64x8.eq`, and `v512.xor` are identical on every host architecture.
- **Full-width native lowering**: v256 uses one YMM operation on amd64; v512
  uses a profitable ZMM operation or two YMM operations. Arm64 uses two or four
  NEON chunks.
- **Checked memory access**: each pointer is validated for the complete vector
  width before any destination bytes are written.
- **Wasm-SIMD semantics**: the catalog mirrors unary, binary, and ternary
  standard and relaxed SIMD kernels at 256- and 512-bit widths.
- **AssemblyScript integration**: the
  [`as-simd`](https://github.com/JairusSW/as-simd) transform emits the imports
  automatically from ordinary `v256` and `v512` source.

> **Stability:** experimental (`v0.1.0`). The plugin ABI and backend selection
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

### On a Runtime

Register the extension before compiling a module that imports Wide
instructions:

```go
package main

import (
	"log"

	"github.com/JairusSW/wide"
	wago "github.com/wago-org/wago"
)

func main() {
	rt := wago.NewRuntime()
	if err := rt.Use(wide.New()); err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	mod, err := rt.Compile(wasmBytes)
	if err != nil {
		log.Fatal(err)
	}

	instance, err := rt.Instantiate(ctx, mod)
	if err != nil {
		log.Fatal(err)
	}
	defer instance.Close()
}
```

Registering the plugin before compilation lets Wago recognize and lower these
imports. Without it, they remain ordinary unresolved Wasm imports. An already
compiled native artifact does not need the plugin to execute.

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

Every operation is an ordinary Wasm import with a pointer-only physical
signature:

| Shape | Wasm signature |
| --- | --- |
| Unary | `(dst: i32, input: i32) -> ()` |
| Binary | `(dst: i32, left: i32, right: i32) -> ()` |
| Ternary | `(dst: i32, a: i32, b: i32, c: i32) -> ()` |

Pointers address 32 bytes for v256 operations and 64 bytes for v512 operations.
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

## Native backends

| Target | v256 | v512 |
| --- | --- | --- |
| amd64 with AVX-512F/DQ/BW | one YMM operation | cost-selected ZMM or two YMM operations |
| amd64 with AVX2 | one YMM operation | two YMM operations |
| arm64 | two NEON chunks | four NEON chunks |

Wago chooses full-width instructions when they reduce work. On CPUs that
execute 512-bit operations as 256-bit halves, its cost model keeps simple
operations on YMM and reserves ZMM for instruction-collapsing wins such as
bitselect and `i64x8.mul`.

Two environment switches support differential testing:

| Variable | Effect |
| --- | --- |
| `WAGO_DISABLE_AVX512=1` | Force the AVX2/YMM v512 fallback |
| `WAGO_FORCE_AVX512=1` | Force every available direct ZMM lowering |

These switches are diagnostic controls, not guest-visible ABI.

## Compatibility

| Axis | Support |
| --- | --- |
| Wago engine | `>= 0.1.0` |
| Go toolchain | `>= 1.22` |
| Guest ABI | ordinary i32 Wasm imports under `"as-simd"` |
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

Run the benchmarks locally:

```sh
go test -run '^$' -bench 'Benchmark(WideSIMDWrapper|V512I64Mul)$' -benchmem
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
- **`plugin_test.go`** - catalog, validation, backend-selection, execution, and
  benchmark coverage.
- **`emitted_integration_test.go`** - optional end-to-end coverage for a real
  `as-simd` transform output.
- **`wago.json`** - package manifest for registry identity, engine
  compatibility, and supported platforms.

Wide owns the semantic import catalog. Wago owns validation, checked addresses,
CPU feature detection, register allocation, and target-specific lowering. This
keeps machine-code access out of the guest ABI and makes the same Wasm module
portable across supported hosts.

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

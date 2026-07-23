// Package wide provides architecture-neutral v256 and v512 instruction
// declarations for Wago. Guest modules use ordinary validated Wasm imports;
// Wide selects the fastest native SIMD lowering available on the host.
package wide

import (
	"fmt"
	"sort"

	wago "github.com/wago-org/wago"
)

const (
	PluginID          = "github.com/JairusSW/wide"
	InstructionModule = "as-simd"
)

type extension struct{ carrier wago.WasmType }

type Option func(*extension)

// WithCarrier selects the standard Wasm type used to validate Wide's
// compiler-erased vector values. Both the guest module and plugin must select
// the same carrier.
func WithCarrier(carrier wago.WasmType) Option {
	return func(ext *extension) { ext.carrier = carrier }
}

func nativeOnlyHandler(name string) wago.InstructionHandler {
	return func(_ wago.InstructionContext, _ []wago.Bits) ([]wago.Bits, error) {
		return nil, fmt.Errorf("as-simd instruction %s requires a native SIMD backend", name)
	}
}

func New(options ...Option) wago.Extension {
	ext := extension{carrier: wago.WasmExternRef}
	for _, option := range options {
		if option != nil {
			option(&ext)
		}
	}
	return ext
}

func (extension) Info() wago.ExtensionInfo {
	return wago.ExtensionInfo{
		ID: PluginID, Name: "Wide", Version: "0.2.0",
		Description: "Portable v256 and v512 instructions with native AVX-512, AVX2, and NEON lowering",
		Stability:   wago.Experimental, License: "MIT",
		Homepage: "https://github.com/JairusSW/wide", Repository: "https://github.com/JairusSW/wide",
		Tags: []string{"simd", "avx512", "avx2", "neon", "assemblyscript", "compiler"},
		Compat: wago.Compatibility{
			Engines:   map[string]string{"wago": ">=0.1.0", "go": ">=1.22", "tinygo": ">=0.41.1"},
			Platforms: []string{"linux/amd64", "linux/arm64"},
		},
	}
}

func (e extension) Register(reg *wago.Registry) error {
	reg.Capability(wago.CapCompilerCodegen, wago.CapabilityDocs("Declares checked architecture-neutral as-simd pointer operations for native SIMD lowering."))
	compiler := reg.Compiler()
	customTypes := make(map[uint16]wago.CustomType, 2)
	for _, bits := range []uint16{256, 512} {
		typ, err := compiler.Type(wago.CustomTypeSpec{
			Name: "wide.v" + itoa(int(bits)), Size: int32(bits / 8), Carrier: e.carrier,
		})
		if err != nil {
			return err
		}
		customTypes[bits] = typ
	}
	subs := make([]int, 0, len(canonicalNames))
	for sub := range canonicalNames {
		subs = append(subs, int(sub))
	}
	sort.Ints(subs)
	for _, rawSub := range subs {
		sub := uint32(rawSub)
		op, _ := operationFor(sub)
		arity, ok := op.kernelArity()
		if !ok {
			continue
		}
		for _, bits := range []uint16{256, 512} {
			customType := customTypes[bits]
			customInputs := make([]wago.CustomType, arity)
			for i := range customInputs {
				customInputs[i] = customType
			}
			width, opcode := bits, sub
			name, _ := instructionName(width, opcode)
			amd64, arm64 := customTargetLowerings(width, opcode, arity)
			err := compiler.Instruction(wago.InstructionSpec{
				Module: InstructionModule, Name: name,
				Custom: &wago.CustomSignature{Inputs: customInputs, Output: &customType},
				AMD64:  amd64,
				ARM64:  arm64,
			})
			if err != nil {
				return err
			}
			memoryInputs := make([]int32, arity+1)
			for i := range memoryInputs {
				memoryInputs[i] = 32
			}
			amd64, arm64 = memoryTargetLowerings(width, opcode, arity)
			if err := compiler.Instruction(wago.InstructionSpec{
				Module: InstructionModule, Name: name + ".memory", Input: memoryInputs,
				Handler: nativeOnlyHandler(name + ".memory"), AMD64: amd64, ARM64: arm64,
			}); err != nil {
				return err
			}
		}
	}
	for _, bits := range []uint16{256, 512} {
		customType := customTypes[bits]
		empty := []wago.CustomType{{}}
		amd64, arm64 := customLoadLowerings(bits)
		if err := compiler.Instruction(wago.InstructionSpec{
			Module: InstructionModule, Name: "v" + itoa(int(bits)) + ".load",
			Input:  []int32{32},
			Custom: &wago.CustomSignature{Inputs: empty, Output: &customType},
			AMD64:  amd64, ARM64: arm64,
		}); err != nil {
			return err
		}
		amd64, arm64 = customStoreLowerings(bits)
		if err := compiler.Instruction(wago.InstructionSpec{
			Module: InstructionModule, Name: "v" + itoa(int(bits)) + ".store",
			Input:  []int32{0, 32},
			Custom: &wago.CustomSignature{Inputs: []wago.CustomType{customType, wago.CustomType{}}},
			AMD64:  amd64, ARM64: arm64,
		}); err != nil {
			return err
		}
	}
	return nil
}

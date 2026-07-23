// Package wide provides architecture-neutral v256 and v512 instruction
// declarations for Wago. Guest modules use ordinary validated i32 imports;
// Wago selects the fastest native SIMD lowering available on the host.
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

type extension struct{}

func New() wago.Extension { return extension{} }

func (extension) Info() wago.ExtensionInfo {
	return wago.ExtensionInfo{
		ID: PluginID, Name: "Wide", Version: "0.1.0",
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

func (extension) Register(reg *wago.Registry) error {
	reg.Capability(wago.CapCompilerCodegen, wago.CapabilityDocs("Declares checked architecture-neutral as-simd pointer operations for native SIMD lowering."))
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
			inputs := make([]int32, arity+1)
			for i := range inputs {
				inputs[i] = 32
			}
			width, opcode := bits, sub
			name, _ := instructionName(width, opcode)
			err := reg.Compiler().Instruction(wago.InstructionSpec{
				Module: InstructionModule, Name: name, Input: inputs,
				Handler: func(_ wago.InstructionContext, _ []wago.Bits) ([]wago.Bits, error) {
					return nil, fmt.Errorf("as-simd instruction %s requires a native SIMD backend", name)
				},
				SIMD: &wago.SIMDInstruction{Width: width, Subopcode: opcode, Arity: uint8(arity)},
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

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

func jsonEscapeCopyHandler(ctx wago.InstructionContext, args []wago.Bits) ([]wago.Bits, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("JSON escape-copy requires source and destination pointers")
	}
	src, dst := uint64(args[0].Uint32()), uint64(args[1].Uint32())
	memory := ctx.Memory()
	if src+64 > uint64(len(memory)) || dst+64 > uint64(len(memory)) {
		return nil, fmt.Errorf("JSON escape-copy memory access is out of bounds")
	}
	copy(memory[dst:dst+64], memory[src:src+64])
	var mask uint32
	for lane := uint32(0); lane < 32; lane++ {
		offset := src + uint64(lane*2)
		code := uint16(memory[offset]) | uint16(memory[offset+1])<<8
		if code == 0x22 || code == 0x5c || code < 0x20 || code >= 0xd800 && code <= 0xdfff {
			mask |= 1 << lane
		}
	}
	result, err := wago.NewBits(32, []byte{
		byte(mask),
		byte(mask >> 8),
		byte(mask >> 16),
		byte(mask >> 24),
	})
	if err != nil {
		return nil, err
	}
	return []wago.Bits{result}, nil
}

func jsonEscapeCopy256Handler(ctx wago.InstructionContext, args []wago.Bits) ([]wago.Bits, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("256-byte JSON escape-copy requires source and destination pointers")
	}
	src, dst := uint64(args[0].Uint32()), uint64(args[1].Uint32())
	memory := ctx.Memory()
	if src+256 > uint64(len(memory)) || dst+256 > uint64(len(memory)) {
		return nil, fmt.Errorf("256-byte JSON escape-copy memory access is out of bounds")
	}
	copy(memory[dst:dst+256], memory[src:src+256])
	var found uint32
	for offset := uint64(0); offset < 256; offset += 2 {
		code := uint16(memory[src+offset]) | uint16(memory[src+offset+1])<<8
		if code == 0x22 || code == 0x5c || code < 0x20 || code >= 0xd800 && code <= 0xdfff {
			found = 1
			break
		}
	}
	result, err := wago.NewBits(32, []byte{byte(found), 0, 0, 0})
	if err != nil {
		return nil, err
	}
	return []wago.Bits{result}, nil
}

func jsonEscapeCopyBulkHandler(ctx wago.InstructionContext, args []wago.Bits) ([]wago.Bits, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("bulk JSON escape-copy requires source, destination, and inclusive end pointers")
	}
	src, dst := uint64(args[0].Uint32()), uint64(args[1].Uint32())
	lastSrc, lastDst := uint64(args[2].Uint32()), uint64(args[3].Uint32())
	memory := ctx.Memory()
	if lastSrc < src || lastDst < dst || lastSrc-src != lastDst-dst ||
		(lastSrc-src)%64 != 0 || lastSrc+64 > uint64(len(memory)) ||
		lastDst+64 > uint64(len(memory)) {
		return nil, fmt.Errorf("bulk JSON escape-copy memory range is invalid")
	}
	var found uint32
	for src <= lastSrc {
		copy(memory[dst:dst+64], memory[src:src+64])
		for offset := uint64(0); offset < 64; offset += 2 {
			code := uint16(memory[src+offset]) | uint16(memory[src+offset+1])<<8
			if code == 0x22 || code == 0x5c || code < 0x20 || code >= 0xd800 && code <= 0xdfff {
				found = 1
			}
		}
		src += 64
		dst += 64
	}
	result, err := wago.NewBits(32, []byte{byte(found), 0, 0, 0})
	if err != nil {
		return nil, err
	}
	return []wago.Bits{result}, nil
}

func jsonFindQuoteBackslashHandler(ctx wago.InstructionContext, args []wago.Bits) ([]wago.Bits, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("JSON quote/backslash scan requires one source pointer")
	}
	src := uint64(args[0].Uint32())
	memory := ctx.Memory()
	if src+64 > uint64(len(memory)) {
		return nil, fmt.Errorf("JSON quote/backslash scan is out of bounds")
	}
	var mask uint32
	for lane := uint32(0); lane < 32; lane++ {
		offset := src + uint64(lane*2)
		code := uint16(memory[offset]) | uint16(memory[offset+1])<<8
		if code == 0x22 || code == 0x5c {
			mask |= 1 << lane
		}
	}
	result, err := wago.NewBits(32, []byte{
		byte(mask), byte(mask >> 8), byte(mask >> 16), byte(mask >> 24),
	})
	if err != nil {
		return nil, err
	}
	return []wago.Bits{result}, nil
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
		ID: PluginID, Name: "Wide", Version: "0.0.0",
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
	if err := compiler.Instruction(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_64",
		Input:   []int32{32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopyHandler,
		AMD64:   jsonEscapeCopyAMD64Lowering(),
	}); err != nil {
		return err
	}
	if err := compiler.Instruction(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_64.v512",
		Input:   []int32{32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopyHandler,
		AMD64:   jsonEscapeCopyAVX512Lowering(),
	}); err != nil {
		return err
	}
	if err := compiler.Instruction(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_256.v512",
		Input:   []int32{32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopy256Handler,
		AMD64:   jsonEscapeCopy256AVX512Lowering(),
	}); err != nil {
		return err
	}
	if err := compiler.Instruction(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_bulk.v512",
		Input:   []int32{32, 32, 32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopyBulkHandler,
		AMD64:   jsonEscapeCopyBulkAVX512Lowering(),
	}); err != nil {
		return err
	}
	if err := compiler.Instruction(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.find_quote_backslash_utf16_64.v512",
		Input:   []int32{32},
		Output:  []int32{32},
		Handler: jsonFindQuoteBackslashHandler,
		AMD64:   jsonFindQuoteBackslashAVX512Lowering(),
	}); err != nil {
		return err
	}
	return nil
}

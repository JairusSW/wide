// Package wide provides architecture-neutral v256 and v512 instruction
// declarations for Wago. Guest modules use ordinary validated Wasm imports;
// Wide selects the fastest native SIMD lowering available on the host.
package wide

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	wago "github.com/wago-org/wago"
)

const (
	PluginID          = "github.com/JairusSW/wide"
	InstructionModule = "as-simd"
)

type plugin struct{ carrier wago.WasmType }

// Config selects the standard Wasm carrier used to validate Wide's
// compiler-erased vector values. Guest modules and the plugin must use the same
// carrier. The empty value defaults to externref.
type Config struct {
	Carrier string `json:"carrier,omitempty"`
}

var configSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "carrier": {
      "type": "string",
      "enum": ["i32", "i64", "f32", "f64", "v128", "funcref", "externref"]
    }
  }
}`)

func decodeConfig(raw json.RawMessage) (Config, wago.WasmType, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := validateConfigObject(raw); err != nil {
		return Config{}, 0, fmt.Errorf("wide: config: %w", err)
	}
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, 0, fmt.Errorf("wide: config: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return Config{}, 0, fmt.Errorf("wide: config has a trailing JSON value")
	}
	carrier, err := carrierForConfig(cfg)
	return cfg, carrier, err
}

func validateConfigObject(raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("must be a JSON object")
	}
	seen := map[string]struct{}{}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("object key is not a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("field %q must not be null", key)
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err == nil {
			return fmt.Errorf("has a trailing JSON value")
		}
		return err
	}
	return nil
}

func carrierForConfig(cfg Config) (wago.WasmType, error) {
	switch cfg.Carrier {
	case "", "externref":
		return wago.WasmExternRef, nil
	case "i32":
		return wago.WasmI32, nil
	case "i64":
		return wago.WasmI64, nil
	case "f32":
		return wago.WasmF32, nil
	case "f64":
		return wago.WasmF64, nil
	case "v128":
		return wago.WasmV128, nil
	case "funcref":
		return wago.WasmFuncRef, nil
	default:
		return 0, fmt.Errorf("wide: unsupported carrier %q", cfg.Carrier)
	}
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

// Definition returns fresh immutable catalog metadata for the Wide provider.
func Definition() wago.PluginDefinition {
	return wago.PluginDefinition{
		ID:          PluginID,
		Name:        "Wide",
		Version:     "0.2.1",
		Description: "Portable v256 and v512 instructions with native AVX-512, AVX2, and NEON lowering.",
		Stability:   wago.Experimental,
		Compatibility: wago.Compatibility{
			Engines: map[string]string{
				"wago": ">=0.1.0", "go": ">=1.22", "tinygo": ">=0.41.1",
			},
			Platforms: []string{
				"darwin/amd64", "darwin/arm64",
				"linux/amd64", "linux/arm64",
				"windows/amd64", "windows/arm64",
			},
		},
		Provenance: wago.PluginProvenance{
			Homepage:   "https://github.com/JairusSW/wide",
			Repository: "https://github.com/JairusSW/wide",
			License:    "MIT",
			Authors:    []string{"Jairus Tanaka"},
		},
		Authorities: []wago.AuthorityRequest{
			{
				Name:   wago.AuthorityCompilerTypeDefine,
				Mode:   wago.AuthorityRequired,
				Reason: "define Wide's compiler-erased vector value types",
				Scope:  wago.AuthorityScope{Modules: []string{"wide"}},
			},
			{
				Name:   wago.AuthorityCompilerInstructionDefine,
				Mode:   wago.AuthorityRequired,
				Reason: "define and lower the as-simd guest instruction ABI",
				Scope:  wago.AuthorityScope{Modules: []string{InstructionModule}},
			},
		},
		ConfigSchema: append(json.RawMessage(nil), configSchema...),
	}
}

// Provider is Wide's explicit, side-effect-free catalog entry.
func Provider() wago.PluginProvider {
	return wago.PluginProvider{
		Definition: Definition(),
		New:        func() wago.Plugin { return new(plugin) },
		ValidateConfig: func(raw json.RawMessage) error {
			_, _, err := decodeConfig(raw)
			return err
		},
	}
}

func (e *plugin) Register(reg *wago.Registrar) error {
	var raw Config
	if err := reg.Config(&raw); err != nil {
		return err
	}
	carrier, err := carrierForConfig(raw)
	if err != nil {
		return err
	}
	e.carrier = carrier
	if err := reg.GuestCapability(wago.CapCompilerCodegen, wago.CapabilityDocs("Declares checked architecture-neutral as-simd pointer operations for native SIMD lowering.")); err != nil {
		return err
	}
	types, err := reg.CompilerTypes()
	if err != nil {
		return err
	}
	instructions, err := reg.CompilerInstructions()
	if err != nil {
		return err
	}
	customTypes := make(map[uint16]wago.CustomType, 2)
	for _, bits := range []uint16{256, 512} {
		typ, err := types.Define(wago.CustomTypeSpec{
			Name: "wide/v" + itoa(int(bits)), Size: int32(bits / 8), Carrier: e.carrier,
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
			lowering := customTargetLowering(width, opcode, arity)
			err := instructions.Define(wago.InstructionSpec{
				Module: InstructionModule, Name: name,
				Custom:  &wago.CustomSignature{Inputs: customInputs, Output: &customType},
				Codegen: lowering,
			})
			if err != nil {
				return err
			}
			memoryInputs := make([]int32, arity+1)
			for i := range memoryInputs {
				memoryInputs[i] = 32
			}
			lowering = memoryTargetLowering(width, opcode, arity)
			if err := instructions.Define(wago.InstructionSpec{
				Module: InstructionModule, Name: name + ".memory", Input: memoryInputs,
				Handler: nativeOnlyHandler(name + ".memory"), Codegen: lowering,
			}); err != nil {
				return err
			}
		}
	}
	for _, bits := range []uint16{256, 512} {
		customType := customTypes[bits]
		empty := []wago.CustomType{{}}
		lowering := customLoadLowering(bits)
		if err := instructions.Define(wago.InstructionSpec{
			Module: InstructionModule, Name: "v" + itoa(int(bits)) + ".load",
			Input:   []int32{32},
			Custom:  &wago.CustomSignature{Inputs: empty, Output: &customType},
			Codegen: lowering,
		}); err != nil {
			return err
		}
		lowering = customStoreLowering(bits)
		if err := instructions.Define(wago.InstructionSpec{
			Module: InstructionModule, Name: "v" + itoa(int(bits)) + ".store",
			Input:   []int32{0, 32},
			Custom:  &wago.CustomSignature{Inputs: []wago.CustomType{customType, wago.CustomType{}}},
			Codegen: lowering,
		}); err != nil {
			return err
		}
	}
	if err := instructions.Define(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_64",
		Input:   []int32{32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopyHandler,
		Codegen: selectTargetLowering(jsonEscapeCopyAMD64Lowering(), nil),
	}); err != nil {
		return err
	}
	if err := instructions.Define(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_64.v512",
		Input:   []int32{32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopyHandler,
		Codegen: selectTargetLowering(jsonEscapeCopyAVX512Lowering(), nil),
	}); err != nil {
		return err
	}
	if err := instructions.Define(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_256.v512",
		Input:   []int32{32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopy256Handler,
		Codegen: selectTargetLowering(jsonEscapeCopy256AVX512Lowering(), nil),
	}); err != nil {
		return err
	}
	if err := instructions.Define(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.escape_copy_utf16_bulk.v512",
		Input:   []int32{32, 32, 32, 32},
		Output:  []int32{32},
		Handler: jsonEscapeCopyBulkHandler,
		Codegen: selectTargetLowering(jsonEscapeCopyBulkAVX512Lowering(), nil),
	}); err != nil {
		return err
	}
	if err := instructions.Define(wago.InstructionSpec{
		Module:  InstructionModule,
		Name:    "json.find_quote_backslash_utf16_64.v512",
		Input:   []int32{32},
		Output:  []int32{32},
		Handler: jsonFindQuoteBackslashHandler,
		Codegen: selectTargetLowering(jsonFindQuoteBackslashAVX512Lowering(), nil),
	}); err != nil {
		return err
	}
	return nil
}

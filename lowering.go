package wide

import (
	"fmt"

	wago "github.com/wago-org/wago"
	x86 "github.com/wago-org/wago/codegen/amd64"
	a64 "github.com/wago-org/wago/codegen/arm64"
)

// targetLowerings is the only seam between Wide's semantic catalog and Wago.
// Every architecture decision stays in this package; Wago only supplies raw
// registers, checked addresses, and encoders.
func memoryTargetLowerings(bits uint16, opcode uint32, arity int) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
	return amd64Lowering(bits, opcode, arity), arm64Lowering(bits, opcode, arity)
}

func virtualTargetLowerings(bits uint16, opcode uint32, arity int) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
	return virtualAMD64Lowering(bits, opcode, arity), virtualARM64Lowering(bits, opcode, arity)
}

func virtualAMD64Lowering(bits uint16, opcode uint32, arity int) *wago.AMD64InstructionLowering {
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX2,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			inputs := make([][]x86.Reg, arity)
			for i := range inputs {
				regs, err := ctx.InputVirtual(i)
				if err != nil {
					return err
				}
				inputs[i] = regs
			}
			chunks := int(bits / 256)
			outputs := make([]x86.Reg, chunks)
			for chunk := 0; chunk < chunks; chunk++ {
				raw := make([]uint8, arity)
				for input := range inputs {
					raw[input] = uint8(inputs[input][chunk])
				}
				out, err := emitAMD64YMM(ctx, opcode, raw)
				if err != nil {
					return err
				}
				outputs[chunk] = out
			}
			if err := ctx.OutputVirtual(outputs...); err != nil {
				return fmt.Errorf("%w (inputs %v)", err, inputs)
			}
			return nil
		},
	}
}

func virtualARM64Lowering(bits uint16, opcode uint32, arity int) *wago.ARM64InstructionLowering {
	return &wago.ARM64InstructionLowering{
		Compatibility: wago.ARM64CompatibilityFullAccess,
		Emit: func(ctx wago.ARM64LoweringContext) error {
			inputs := make([][]a64.Reg, arity)
			seen := map[a64.Reg]bool{}
			for i := range inputs {
				regs, err := ctx.InputVirtual(i)
				if err != nil {
					return err
				}
				for _, reg := range regs {
					if seen[reg] {
						return fmt.Errorf("wide: arm64 virtual input bundles alias at register %d: %v", reg, inputs)
					}
					seen[reg] = true
				}
				inputs[i] = regs
			}
			chunks := int(bits / 128)
			outputs := make([]a64.Reg, chunks)
			for chunk := 0; chunk < chunks; chunk++ {
				raw := make([]uint8, arity)
				for input := range inputs {
					raw[input] = uint8(inputs[input][chunk])
				}
				out, err := emitARM64NEON(ctx, opcode, raw)
				if err != nil {
					return err
				}
				outputs[chunk] = out
			}
			if err := ctx.OutputVirtual(outputs...); err != nil {
				return fmt.Errorf("%w (inputs %v)", err, inputs)
			}
			return nil
		},
	}
}

func virtualLoadLowerings(bits uint16) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
	amd64 := &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX2,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			base, index, disp, err := ctx.CheckedMemory(0, 0, int(bits/8))
			if err != nil {
				return err
			}
			outputs := make([]x86.Reg, bits/256)
			for i := range outputs {
				reg := ctx.AllocYMM()
				ctx.Encoder().YMovdquLoadIdx(reg, base, index, disp+int32(i*32))
				outputs[i] = reg
			}
			ctx.ReleaseGP(index)
			return ctx.OutputVirtual(outputs...)
		},
	}
	arm64 := &wago.ARM64InstructionLowering{
		Compatibility: wago.ARM64CompatibilityFullAccess,
		Emit: func(ctx wago.ARM64LoweringContext) error {
			base, index, disp, err := ctx.CheckedMemory(0, 0, int(bits/8))
			if err != nil {
				return err
			}
			outputs := make([]a64.Reg, bits/128)
			for i := range outputs {
				reg := ctx.AllocVector()
				ctx.Encoder().LdrQIdx(reg, base, index, disp+int32(i*16))
				outputs[i] = reg
			}
			ctx.ReleaseGP(index)
			return ctx.OutputVirtual(outputs...)
		},
	}
	return amd64, arm64
}

func virtualStoreLowerings(bits uint16) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
	amd64 := &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX2,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			values, err := ctx.InputVirtual(0)
			if err != nil {
				return err
			}
			base, index, disp, err := ctx.CheckedMemory(1, 0, int(bits/8))
			if err != nil {
				return err
			}
			for i, value := range values {
				ctx.Encoder().YMovdquStoreIdx(base, index, value, disp+int32(i*32))
				ctx.ReleaseVector(value)
			}
			ctx.ReleaseGP(index)
			return nil
		},
	}
	arm64 := &wago.ARM64InstructionLowering{
		Compatibility: wago.ARM64CompatibilityFullAccess,
		Emit: func(ctx wago.ARM64LoweringContext) error {
			values, err := ctx.InputVirtual(0)
			if err != nil {
				return err
			}
			base, index, disp, err := ctx.CheckedMemory(1, 0, int(bits/8))
			if err != nil {
				return err
			}
			for i, value := range values {
				ctx.Encoder().StrQIdx(base, index, value, disp+int32(i*16))
				ctx.ReleaseVector(value)
			}
			ctx.ReleaseGP(index)
			return nil
		},
	}
	return amd64, arm64
}

func amd64Lowering(bits uint16, opcode uint32, arity int) *wago.AMD64InstructionLowering {
	useZMM := bits == 512 && shouldUseZMM(opcode)
	features := wago.AMD64FeatureAVX2
	if useZMM {
		features = wago.AMD64FeatureAVX512
	}
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      features,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			chunk := amd64ChunkBytes(opcode)
			if useZMM {
				chunk = 64
			}
			type address struct {
				base, index x86.Reg
				disp        int32
			}
			addresses := make([]address, arity+1)
			for i := range addresses {
				base, index, disp, err := ctx.CheckedMemory(i, 0, int(bits/8))
				if err != nil {
					return err
				}
				addresses[i] = address{base: base, index: index, disp: disp}
			}
			for offset := uint32(0); offset < uint32(bits/8); offset += chunk {
				inputs := make([]uint8, arity)
				for i := range inputs {
					src := addresses[i+1]
					reg := ctx.AllocYMM()
					if chunk == 64 {
						ctx.Encoder().ZMovdqu64LoadIdx(reg, src.base, src.index, src.disp+int32(offset))
					} else if chunk == 32 {
						ctx.Encoder().YMovdquLoadIdx(reg, src.base, src.index, src.disp+int32(offset))
					} else {
						ctx.Encoder().VMovdquLoadIdx(reg, src.base, src.index, src.disp+int32(offset))
					}
					inputs[i] = uint8(reg)
				}
				var out x86.Reg
				var err error
				if useZMM {
					out, err = emitAMD64ZMM(ctx, opcode, inputs)
				} else {
					out, err = emitAMD64YMM(ctx, opcode, inputs)
				}
				if err != nil {
					return err
				}
				dst := addresses[0]
				if chunk == 64 {
					ctx.Encoder().ZMovdqu64StoreIdx(dst.base, dst.index, out, dst.disp+int32(offset))
				} else if chunk == 32 {
					ctx.Encoder().YMovdquStoreIdx(dst.base, dst.index, out, dst.disp+int32(offset))
				} else {
					ctx.Encoder().VMovdquStoreIdx(dst.base, dst.index, out, dst.disp+int32(offset))
				}
				ctx.ReleaseVector(out)
			}
			return nil
		},
	}
}

func arm64Lowering(bits uint16, opcode uint32, arity int) *wago.ARM64InstructionLowering {
	return &wago.ARM64InstructionLowering{
		Compatibility: wago.ARM64CompatibilityFullAccess,
		Emit: func(ctx wago.ARM64LoweringContext) error {
			type address struct {
				base, index a64.Reg
				disp        int32
			}
			addresses := make([]address, arity+1)
			for i := range addresses {
				base, index, disp, err := ctx.CheckedMemory(i, 0, int(bits/8))
				if err != nil {
					return err
				}
				addresses[i] = address{base: base, index: index, disp: disp}
			}
			for offset := uint32(0); offset < uint32(bits/8); offset += 16 {
				inputs := make([]uint8, arity)
				for i := range inputs {
					src := addresses[i+1]
					v := ctx.AllocVector()
					ctx.Encoder().LdrQIdx(v, src.base, src.index, src.disp+int32(offset))
					inputs[i] = uint8(v)
				}
				out, err := emitARM64NEON(ctx, opcode, inputs)
				if err != nil {
					return err
				}
				dst := addresses[0]
				ctx.Encoder().StrQIdx(dst.base, dst.index, out, dst.disp+int32(offset))
				ctx.ReleaseVector(out)
			}
			return nil
		},
	}
}

func unsupportedTarget(target string, opcode uint32) error {
	name := canonicalNames[opcode]
	return fmt.Errorf("wide: %s lowering for %s (%d) is not implemented", target, name, opcode)
}

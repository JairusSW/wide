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
func targetLowerings(bits uint16, opcode uint32, arity int) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
	return amd64Lowering(bits, opcode, arity), arm64Lowering(bits, opcode, arity)
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
				ctx.Release(out)
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
				ctx.Release(out)
			}
			return nil
		},
	}
}

func unsupportedTarget(target string, opcode uint32) error {
	name := canonicalNames[opcode]
	return fmt.Errorf("wide: %s lowering for %s (%d) is not implemented", target, name, opcode)
}

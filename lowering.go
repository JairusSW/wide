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

func customTargetLowerings(bits uint16, opcode uint32, arity int) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
	return customAMD64Lowering(bits, opcode, arity), customARM64Lowering(bits, opcode, arity)
}

func customAMD64Lowering(bits uint16, opcode uint32, arity int) *wago.AMD64InstructionLowering {
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX2,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			inputs := make([][]x86.Reg, arity)
			for i := range inputs {
				regs, err := ctx.InputCustom(i)
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
			if err := ctx.OutputCustom(outputs...); err != nil {
				return fmt.Errorf("%w (inputs %v)", err, inputs)
			}
			return nil
		},
	}
}

func customARM64Lowering(bits uint16, opcode uint32, arity int) *wago.ARM64InstructionLowering {
	return &wago.ARM64InstructionLowering{
		Compatibility: wago.ARM64CompatibilityFullAccess,
		Emit: func(ctx wago.ARM64LoweringContext) error {
			inputs := make([][]a64.Reg, arity)
			seen := map[a64.Reg]bool{}
			for i := range inputs {
				regs, err := ctx.InputCustom(i)
				if err != nil {
					return err
				}
				for _, reg := range regs {
					if seen[reg] {
						return fmt.Errorf("wide: arm64 custom input bundles alias at register %d: %v", reg, inputs)
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
			if err := ctx.OutputCustom(outputs...); err != nil {
				return fmt.Errorf("%w (inputs %v)", err, inputs)
			}
			return nil
		},
	}
}

func customLoadLowerings(bits uint16) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
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
			return ctx.OutputCustom(outputs...)
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
			return ctx.OutputCustom(outputs...)
		},
	}
	return amd64, arm64
}

func customStoreLowerings(bits uint16) (*wago.AMD64InstructionLowering, *wago.ARM64InstructionLowering) {
	amd64 := &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX2,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			values, err := ctx.InputCustom(0)
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
			values, err := ctx.InputCustom(0)
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

func jsonEscapeCopyAMD64Lowering() *wago.AMD64InstructionLowering {
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX2,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			var result x86.Reg
			for chunk := uint32(0); chunk < 2; chunk++ {
				offset := chunk * 32
				value, err := ctx.LoadYMM(0, offset)
				if err != nil {
					return err
				}
				if err := ctx.StoreYMM(1, offset, value); err != nil {
					ctx.ReleaseVector(value)
					return err
				}
				mask := jsonEscapeMaskYMM(ctx, value)
				if chunk == 0 {
					result = mask
				} else {
					ctx.Encoder().Or32(result, mask)
					ctx.ReleaseGP(mask)
				}
			}
			return ctx.OutputI32(result)
		},
	}
}

func jsonEscapeCopyAVX512Lowering() *wago.AMD64InstructionLowering {
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX512,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			const (
				value = x86.Reg(0)
				splat = x86.Reg(1)
				temp  = x86.Reg(2)
			)
			if err := ctx.ReserveYMM(value); err != nil {
				return err
			}
			if err := ctx.ReserveYMM(splat); err != nil {
				return err
			}
			if err := ctx.ReserveYMM(temp); err != nil {
				return err
			}
			mask := ctx.AllocGP()

			srcBase, srcIndex, srcDisp, err := ctx.CheckedMemory(0, 0, 64)
			if err != nil {
				return err
			}
			dstBase, dstIndex, dstDisp, err := ctx.CheckedMemory(1, 0, 64)
			if err != nil {
				ctx.ReleaseGP(srcIndex)
				return err
			}
			a := ctx.Encoder()
			a.ZMovdqu64LoadIdx(value, srcBase, srcIndex, srcDisp)
			a.ZMovdqu64StoreIdx(dstBase, dstIndex, value, dstDisp)
			ctx.ReleaseGP(srcIndex)
			ctx.ReleaseGP(dstIndex)

			// vpbroadcastw zmm1,r32
			broadcast := func() {
				p0 := byte(0xf2)
				if mask >= 8 {
					p0 &^= 0x20
				}
				a.B = append(a.B, 0x62, p0, 0x7d, 0x48, 0x7b, 0xc8|byte(mask&7))
			}
			a.MovImm32(mask, 0x22)
			broadcast()
			// vpcmpeqw k1,zmm0,zmm1
			a.B = append(a.B, 0x62, 0xf1, 0x7d, 0x48, 0x75, 0xc9)

			a.MovImm32(mask, 0x5c)
			broadcast()
			// vpcmpeqw k2,zmm0,zmm1
			a.B = append(a.B, 0x62, 0xf1, 0x7d, 0x48, 0x75, 0xd1)

			a.MovImm32(mask, 0x20)
			broadcast()
			// vpcmpuw k3,zmm0,zmm1,lt
			a.B = append(a.B, 0x62, 0xf3, 0xfd, 0x48, 0x3e, 0xd9, 0x01)

			a.MovImm32(mask, 0xd800)
			broadcast()
			// vpsubw zmm2,zmm0,zmm1
			a.B = append(a.B, 0x62, 0xf1, 0x7d, 0x48, 0xf9, 0xd1)
			a.MovImm32(mask, 0x800)
			broadcast()
			// vpcmpuw k4,zmm2,zmm1,lt
			a.B = append(a.B, 0x62, 0xf3, 0xed, 0x48, 0x3e, 0xe1, 0x01)

			// kord k1,k1,k2; kord k1,k1,k3; kord k1,k1,k4
			a.B = append(a.B,
				0xc4, 0xe1, 0xf5, 0x45, 0xca,
				0xc4, 0xe1, 0xf5, 0x45, 0xcb,
				0xc4, 0xe1, 0xf5, 0x45, 0xcc,
			)
			// kmovd r32,k1
			vex := byte(0xfb)
			if mask >= 8 {
				vex &^= 0x80
			}
			a.B = append(a.B, 0xc5, vex, 0x93, 0xc1|byte(mask&7)<<3)

			ctx.ReleaseVector(value)
			ctx.ReleaseVector(splat)
			ctx.ReleaseVector(temp)
			return ctx.OutputI32(mask)
		},
	}
}

func jsonEscapeCopy256AVX512Lowering() *wago.AMD64InstructionLowering {
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX512,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			for reg := x86.Reg(0); reg <= 6; reg++ {
				if err := ctx.ReserveYMM(reg); err != nil {
					return err
				}
			}
			mask := ctx.AllocGP()
			srcBase, srcIndex, srcDisp, err := ctx.CheckedMemory(0, 0, 256)
			if err != nil {
				return err
			}
			dstBase, dstIndex, dstDisp, err := ctx.CheckedMemory(1, 0, 256)
			if err != nil {
				ctx.ReleaseGP(srcIndex)
				return err
			}
			a := ctx.Encoder()
			broadcast := func(dst byte) {
				p0 := byte(0xf2)
				if mask >= 8 {
					p0 &^= 0x20
				}
				a.B = append(
					a.B,
					0x62,
					p0,
					0x7d,
					0x48,
					0x7b,
					0xc0|(dst<<3)|byte(mask&7),
				)
			}
			a.MovImm32(mask, 0x22)
			broadcast(1)
			a.MovImm32(mask, 0x5c)
			broadcast(2)
			a.MovImm32(mask, 0x20)
			broadcast(3)
			a.MovImm32(mask, 0xd800)
			broadcast(4)
			a.MovImm32(mask, 0x800)
			broadcast(5)
			// kxord k1,k1,k1
			a.B = append(a.B, 0xc4, 0xe1, 0xf5, 0x47, 0xc9)

			for offset := int32(0); offset < 256; offset += 64 {
				a.ZMovdqu64LoadIdx(0, srcBase, srcIndex, srcDisp+offset)
				a.ZMovdqu64StoreIdx(dstBase, dstIndex, 0, dstDisp+offset)
				// Four predicates, surrogate range reduction, and mask merge.
				a.B = append(a.B,
					0x62, 0xf1, 0x7d, 0x48, 0x75, 0xd1,
					0x62, 0xf1, 0x7d, 0x48, 0x75, 0xda,
					0x62, 0xf3, 0xfd, 0x48, 0x3e, 0xe3, 0x01,
					0x62, 0xf1, 0x7d, 0x48, 0xf9, 0xf4,
					0x62, 0xf3, 0xcd, 0x48, 0x3e, 0xed, 0x01,
					0xc4, 0xe1, 0xed, 0x45, 0xd3,
					0xc4, 0xe1, 0xed, 0x45, 0xd4,
					0xc4, 0xe1, 0xed, 0x45, 0xd5,
					0xc4, 0xe1, 0xf5, 0x45, 0xca,
				)
			}
			ctx.ReleaseGP(srcIndex)
			ctx.ReleaseGP(dstIndex)
			vex := byte(0xfb)
			if mask >= 8 {
				vex &^= 0x80
			}
			a.B = append(a.B, 0xc5, vex, 0x93, 0xc1|byte(mask&7)<<3)
			for reg := x86.Reg(0); reg <= 6; reg++ {
				ctx.ReleaseVector(reg)
			}
			return ctx.OutputI32(mask)
		},
	}
}

func jsonEscapeCopyBulkAVX512Lowering() *wago.AMD64InstructionLowering {
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX512,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			for reg := x86.Reg(0); reg <= 6; reg++ {
				if err := ctx.ReserveYMM(reg); err != nil {
					return err
				}
			}

			// Validate the inclusive final blocks once. The loop below uses the
			// trusted raw pointer registers after these Wasm bounds checks.
			_, lastSrcIndex, _, err := ctx.CheckedMemory(2, 0, 64)
			if err != nil {
				return err
			}
			ctx.ReleaseGP(lastSrcIndex)
			_, lastDstIndex, _, err := ctx.CheckedMemory(3, 0, 64)
			if err != nil {
				return err
			}
			ctx.ReleaseGP(lastDstIndex)

			src, err := ctx.InputI32(0)
			if err != nil {
				return err
			}
			dst, err := ctx.InputI32(1)
			if err != nil {
				return err
			}
			lastSrc, err := ctx.InputI32(2)
			if err != nil {
				return err
			}
			scalar := ctx.AllocGP(src, dst, lastSrc)
			a := ctx.Encoder()
			broadcast := func(dst byte) {
				p0 := byte(0xf2)
				if scalar >= 8 {
					p0 &^= 0x20
				}
				a.B = append(
					a.B,
					0x62,
					p0,
					0x7d,
					0x48,
					0x7b,
					0xc0|(dst<<3)|byte(scalar&7),
				)
			}
			a.MovImm32(scalar, 0x22)
			broadcast(1)
			a.MovImm32(scalar, 0x5c)
			broadcast(2)
			a.MovImm32(scalar, 0x20)
			broadcast(3)
			a.MovImm32(scalar, 0xd800)
			broadcast(4)
			a.MovImm32(scalar, 0x800)
			broadcast(5)
			// kxord k1,k1,k1
			a.B = append(a.B, 0xc4, 0xe1, 0xf5, 0x47, 0xc9)

			loop := a.Len()
			a.ZMovdqu64LoadIdx(0, ctx.MemoryBase(), src, 0)
			a.ZMovdqu64StoreIdx(ctx.MemoryBase(), dst, 0, 0)
			a.B = append(a.B,
				// quote, backslash, control and surrogate predicates
				0x62, 0xf1, 0x7d, 0x48, 0x75, 0xd1,
				0x62, 0xf1, 0x7d, 0x48, 0x75, 0xda,
				0x62, 0xf3, 0xfd, 0x48, 0x3e, 0xe3, 0x01,
				0x62, 0xf1, 0x7d, 0x48, 0xf9, 0xf4,
				0x62, 0xf3, 0xcd, 0x48, 0x3e, 0xed, 0x01,
				// merge the block into aggregate k1
				0xc4, 0xe1, 0xed, 0x45, 0xd3,
				0xc4, 0xe1, 0xed, 0x45, 0xd4,
				0xc4, 0xe1, 0xed, 0x45, 0xd5,
				0xc4, 0xe1, 0xf5, 0x45, 0xca,
			)
			a.AluRI(0, src, 64, false)
			a.AluRI(0, dst, 64, false)
			a.Cmp32(src, lastSrc)
			back := a.JccPlaceholder(x86.CondBE)
			a.PatchRel32(back, loop)

			vex := byte(0xfb)
			if scalar >= 8 {
				vex &^= 0x80
			}
			a.B = append(a.B, 0xc5, vex, 0x93, 0xc1|byte(scalar&7)<<3)
			ctx.ReleaseGP(src)
			ctx.ReleaseGP(dst)
			ctx.ReleaseGP(lastSrc)
			for reg := x86.Reg(0); reg <= 6; reg++ {
				ctx.ReleaseVector(reg)
			}
			return ctx.OutputI32(scalar)
		},
	}
}

func jsonFindQuoteBackslashAVX512Lowering() *wago.AMD64InstructionLowering {
	return &wago.AMD64InstructionLowering{
		Compatibility: wago.AMD64CompatibilityFullAccess,
		Features:      wago.AMD64FeatureAVX512,
		Emit: func(ctx wago.AMD64LoweringContext) error {
			const (
				value = x86.Reg(0)
				splat = x86.Reg(1)
			)
			if err := ctx.ReserveYMM(value); err != nil {
				return err
			}
			if err := ctx.ReserveYMM(splat); err != nil {
				return err
			}
			result := ctx.AllocGP()
			base, index, disp, err := ctx.CheckedMemory(0, 0, 64)
			if err != nil {
				return err
			}
			a := ctx.Encoder()
			a.ZMovdqu64LoadIdx(value, base, index, disp)
			ctx.ReleaseGP(index)
			broadcast := func() {
				p0 := byte(0xf2)
				if result >= 8 {
					p0 &^= 0x20
				}
				a.B = append(a.B, 0x62, p0, 0x7d, 0x48, 0x7b, 0xc8|byte(result&7))
			}
			a.MovImm32(result, 0x22)
			broadcast()
			// vpcmpeqw k1,zmm0,zmm1
			a.B = append(a.B, 0x62, 0xf1, 0x7d, 0x48, 0x75, 0xc9)
			a.MovImm32(result, 0x5c)
			broadcast()
			// vpcmpeqw k2,zmm0,zmm1; kord k1,k1,k2
			a.B = append(a.B,
				0x62, 0xf1, 0x7d, 0x48, 0x75, 0xd1,
				0xc4, 0xe1, 0xf5, 0x45, 0xca,
			)
			vex := byte(0xfb)
			if result >= 8 {
				vex &^= 0x80
			}
			a.B = append(a.B, 0xc5, vex, 0x93, 0xc1|byte(result&7)<<3)
			ctx.ReleaseVector(value)
			ctx.ReleaseVector(splat)
			return ctx.OutputI32(result)
		},
	}
}

func jsonEscapeMaskYMM(ctx wago.AMD64LoweringContext, value x86.Reg) x86.Reg {
	a := ctx.Encoder()
	eq := ctx.ConstYMMRepeated128(0x0022002200220022, 0x0022002200220022)
	a.YPcmpeqw(eq, value, eq)
	slash := ctx.ConstYMMRepeated128(0x005c005c005c005c, 0x005c005c005c005c)
	a.YPcmpeqw(slash, value, slash)
	a.YPor(eq, eq, slash)
	ctx.ReleaseVector(slash)

	lt := ctx.AllocYMM(value, eq)
	bias16 := ctx.ConstYMMRepeated128(0x8000800080008000, 0x8000800080008000)
	a.YPxor(lt, value, bias16)
	ctx.ReleaseVector(bias16)
	limit := ctx.ConstYMMRepeated128(0x8020802080208020, 0x8020802080208020)
	a.YPcmpgtw(lt, limit, lt)
	ctx.ReleaseVector(limit)
	a.YPor(eq, eq, lt)
	ctx.ReleaseVector(lt)

	bias8 := ctx.ConstYMMRepeated128(0x8080808080808080, 0x8080808080808080)
	a.YPxor(value, value, bias8)
	ctx.ReleaseVector(bias8)
	pattern := ctx.ConstYMMRepeated128(0x577e577e577e577e, 0x577e577e577e577e)
	a.YPcmpgtb(value, value, pattern)
	ctx.ReleaseVector(pattern)
	a.YPor(eq, eq, value)
	ctx.ReleaseVector(value)

	mask := ctx.AllocGP()
	a.YPmovmskb(mask, eq)
	ctx.ReleaseVector(eq)
	return mask
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

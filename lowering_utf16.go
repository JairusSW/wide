package wide

import x86 "github.com/wago-org/wago/codegen/amd64"

// utf16ValidateBlockAMD64Lowering checks surrogate pairing across a full
// 256/512-bit block. The pointer includes the previous UTF-16 code unit.
func utf16ValidateBlockAMD64Lowering(width uint32) *x86.Lowering {
	return &x86.Lowering{
		Compatibility: x86.CompatibilityFullAccess, Features: x86.FeatureAVX2,
		Emit: func(ctx x86.Context) error {
			_, index, _, err := ctx.CheckedMemory(0, 0, int(width+2))
			if err != nil {
				return err
			}
			ctx.ReleaseGP(index)
			src, err := ctx.InputI32(0)
			if err != nil {
				return err
			}
			acc := ctx.AllocGP(src)
			part := ctx.AllocGP(src, acc)
			a := ctx.Encoder()
			a.MovImm32(acc, 0)
			const fc00 uint64 = 0xfc00fc00fc00fc00
			const d800 uint64 = 0xd800d800d800d800
			const dc00 uint64 = 0xdc00dc00dc00dc00
			for offset := uint32(0); offset < width; offset += 32 {
				current := ctx.AllocYMM()
				previous := ctx.AllocYMM(current)
				low := ctx.AllocYMM(current, previous)
				high := ctx.AllocYMM(current, previous, low)
				a.YMovdquLoadIdx(current, ctx.MemoryBase(), src, int32(offset+2))
				a.YMovdquLoadIdx(previous, ctx.MemoryBase(), src, int32(offset))
				mask := ctx.ConstYMMRepeated128(fc00, fc00)
				a.YPand(low, current, mask)
				a.YPand(high, previous, mask)
				ctx.ReleaseVector(mask)
				lo := ctx.ConstYMMRepeated128(dc00, dc00)
				a.YPcmpeqw(low, low, lo)
				ctx.ReleaseVector(lo)
				hi := ctx.ConstYMMRepeated128(d800, d800)
				a.YPcmpeqw(high, high, hi)
				ctx.ReleaseVector(hi)
				a.YPxor(low, low, high)
				a.YPmovmskb(part, low)
				a.Or32(acc, part)
				for _, r := range []x86.Reg{high, low, previous, current} {
					ctx.ReleaseVector(r)
				}
			}
			ctx.ReleaseGP(src)
			ctx.ReleaseGP(part)
			return ctx.OutputI32(acc)
		},
	}
}

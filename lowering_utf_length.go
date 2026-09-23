package wide

import x86 "github.com/wago-org/wago/codegen/amd64"

func utf16LengthUTF8BlockAMD64Lowering(width uint32) *x86.Lowering {
	return &x86.Lowering{Compatibility: x86.CompatibilityFullAccess, Features: x86.FeatureAVX2, Emit: func(ctx x86.Context) error {
		_, index, _, err := ctx.CheckedMemory(0, 0, int(width))
		if err != nil {
			return err
		}
		ctx.ReleaseGP(index)
		src, err := ctx.InputI32(0)
		if err != nil {
			return err
		}
		total := ctx.AllocGP(src)
		bits := ctx.AllocGP(src, total)
		count := ctx.AllocGP(src, total, bits)
		a := ctx.Encoder()
		a.MovImm32(total, int32(width))
		rep := func(b byte) uint64 { return uint64(b) * 0x0101010101010101 }
		for offset := uint32(0); offset < width; offset += 32 {
			in := ctx.AllocYMM()
			tmp := ctx.AllocYMM(in)
			a.YMovdquLoadIdx(in, ctx.MemoryBase(), src, int32(offset))
			c := ctx.ConstYMMRepeated128(rep(0xc0), rep(0xc0))
			a.YPand(tmp, in, c)
			ctx.ReleaseVector(c)
			c = ctx.ConstYMMRepeated128(rep(0x80), rep(0x80))
			a.YPcmpeqb(tmp, tmp, c)
			ctx.ReleaseVector(c)
			a.YPmovmskb(bits, tmp)
			a.Popcnt(count, bits, false)
			a.Sub32(total, count)
			c = ctx.ConstYMMRepeated128(rep(0xf8), rep(0xf8))
			a.YPand(tmp, in, c)
			ctx.ReleaseVector(c)
			c = ctx.ConstYMMRepeated128(rep(0xf0), rep(0xf0))
			a.YPcmpeqb(tmp, tmp, c)
			ctx.ReleaseVector(c)
			a.YPmovmskb(bits, tmp)
			a.Popcnt(count, bits, false)
			a.Add32(total, count)
			ctx.ReleaseVector(tmp)
			ctx.ReleaseVector(in)
		}
		ctx.ReleaseGP(src)
		ctx.ReleaseGP(bits)
		ctx.ReleaseGP(count)
		return ctx.OutputI32(total)
	}}
}

func utf8LengthUTF16BlockAMD64Lowering(width uint32) *x86.Lowering {
	return &x86.Lowering{Compatibility: x86.CompatibilityFullAccess, Features: x86.FeatureAVX2, Emit: func(ctx x86.Context) error {
		_, index, _, err := ctx.CheckedMemory(0, 0, int(width))
		if err != nil {
			return err
		}
		ctx.ReleaseGP(index)
		src, err := ctx.InputI32(0)
		if err != nil {
			return err
		}
		total := ctx.AllocGP(src)
		bits := ctx.AllocGP(src, total)
		count := ctx.AllocGP(src, total, bits)
		a := ctx.Encoder()
		a.MovImm32(total, int32(width/2))
		const biasBits uint64 = 0x8000800080008000
		for offset := uint32(0); offset < width; offset += 32 {
			in := ctx.AllocYMM()
			tmp := ctx.AllocYMM(in)
			a.YMovdquLoadIdx(in, ctx.MemoryBase(), src, int32(offset))
			bias := ctx.ConstYMMRepeated128(biasBits, biasBits)
			ge := func(threshold uint16) {
				cval := uint64((threshold - 1) ^ 0x8000)
				cval |= cval << 16
				cval |= cval << 32
				c := ctx.ConstYMMRepeated128(cval, cval)
				a.YPxor(tmp, in, bias)
				a.YPcmpgtw(tmp, tmp, c)
				ctx.ReleaseVector(c)
				a.YPmovmskb(bits, tmp)
				a.Popcnt(count, bits, false)
				a.ShiftImm(5, count, 1, false)
				a.Add32(total, count)
			}
			ge(0x80)
			ge(0x800)
			ctx.ReleaseVector(bias)
			c := ctx.ConstYMMRepeated128(0xfc00fc00fc00fc00, 0xfc00fc00fc00fc00)
			a.YPand(tmp, in, c)
			ctx.ReleaseVector(c)
			c = ctx.ConstYMMRepeated128(0xdc00dc00dc00dc00, 0xdc00dc00dc00dc00)
			a.YPcmpeqw(tmp, tmp, c)
			ctx.ReleaseVector(c)
			a.YPmovmskb(bits, tmp)
			a.Popcnt(count, bits, false)
			a.Sub32(total, count)
			ctx.ReleaseVector(tmp)
			ctx.ReleaseVector(in)
		}
		ctx.ReleaseGP(src)
		ctx.ReleaseGP(bits)
		ctx.ReleaseGP(count)
		return ctx.OutputI32(total)
	}}
}

package wide

import (
	"encoding/binary"
	x86 "github.com/wago-org/wago/codegen/amd64"
)

// utf8ValidateBlockAMD64Lowering checks every byte of one 32/64-byte block.
// The pointer addresses three preceding bytes followed by the block. Shifted
// YMM loads make UTF-8 sequences crossing a vector boundary ordinary lanes.
func utf8ValidateBlockAMD64Lowering(width uint32) *x86.Lowering {
	return &x86.Lowering{
		Compatibility: x86.CompatibilityFullAccess,
		Features:      x86.FeatureAVX2,
		Emit: func(ctx x86.Context) error {
			_, index, _, err := ctx.CheckedMemory(0, 0, int(width+3))
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
			rep := func(b byte) uint64 { return uint64(b) * 0x0101010101010101 }

			// Keiser-Lemire nibble tables, repeated independently in each
			// 128-bit lane of the AVX2 byte shuffle.
			const tooShort, tooLong, over3, tooLarge, surrogate, over2, over4, twoCont byte = 1, 2, 4, 8, 16, 32, 64, 128
			carry := tooShort | tooLong | twoCont
			table1 := [16]byte{tooLong, tooLong, tooLong, tooLong, tooLong, tooLong, tooLong, tooLong, twoCont, twoCont, twoCont, twoCont, tooShort | over2, tooShort, tooShort | over3 | surrogate, tooShort | tooLarge | over4}
			table2 := [16]byte{carry | over3 | over2 | over4, carry | over2, carry, carry, carry | tooLarge, carry | tooLarge | over4, carry | tooLarge | over4, carry | tooLarge | over4, carry | tooLarge | over4, carry | tooLarge | over4, carry | tooLarge | over4, carry | tooLarge | over4, carry | tooLarge | over4, carry | tooLarge | over4 | surrogate, carry | tooLarge | over4, carry | tooLarge | over4}
			table3 := [16]byte{tooShort, tooShort, tooShort, tooShort, tooShort, tooShort, tooShort, tooShort, tooLong | over2 | twoCont | over3 | over4, tooLong | over2 | twoCont | over3 | tooLarge, tooLong | over2 | twoCont | surrogate | tooLarge, tooLong | over2 | twoCont | surrogate | tooLarge, tooShort, tooShort, tooShort, tooShort}
			lut := func(table [16]byte) x86.Reg {
				return ctx.ConstYMMRepeated128(binary.LittleEndian.Uint64(table[:8]), binary.LittleEndian.Uint64(table[8:]))
			}
			for offset := uint32(0); offset < width; offset += 32 {
				in := ctx.AllocYMM()
				p1 := ctx.AllocYMM(in)
				p2 := ctx.AllocYMM(in, p1)
				p3 := ctx.AllocYMM(in, p1, p2)
				sc := ctx.AllocYMM(in, p1, p2, p3)
				tmp := ctx.AllocYMM(in, p1, p2, p3, sc)
				mask0f := ctx.ConstYMMRepeated128(rep(0x0f), rep(0x0f))
				load := func(dst x86.Reg, delta int32) { a.YMovdquLoadIdx(dst, ctx.MemoryBase(), src, int32(offset)+delta) }
				load(in, 3)
				load(p1, 2)
				load(p2, 1)
				load(p3, 0)
				a.YPsrlwImm(tmp, p1, 4)
				a.YPand(tmp, tmp, mask0f)
				t := lut(table1)
				a.YPshufb(sc, t, tmp)
				ctx.ReleaseVector(t)
				a.YPand(p1, p1, mask0f)
				t = lut(table2)
				a.YPshufb(p1, t, p1)
				ctx.ReleaseVector(t)
				a.YPsrlwImm(tmp, in, 4)
				a.YPand(tmp, tmp, mask0f)
				t = lut(table3)
				a.YPshufb(tmp, t, tmp)
				ctx.ReleaseVector(t)
				a.YPand(sc, sc, p1)
				a.YPand(sc, sc, tmp)
				t = ctx.ConstYMMRepeated128(rep(0x60), rep(0x60))
				a.YPsubusb(p2, p2, t)
				ctx.ReleaseVector(t)
				t = ctx.ConstYMMRepeated128(rep(0x70), rep(0x70))
				a.YPsubusb(p3, p3, t)
				ctx.ReleaseVector(t)
				a.YPor(p2, p2, p3)
				t = ctx.ConstYMMRepeated128(rep(0x80), rep(0x80))
				a.YPand(p2, p2, t)
				ctx.ReleaseVector(t)
				a.YPxor(sc, sc, p2)
				a.YPxor(tmp, tmp, tmp)
				a.YPcmpeqb(sc, sc, tmp)
				a.YPcmpeqb(tmp, tmp, tmp)
				a.YPxor(sc, sc, tmp)
				a.YPmovmskb(part, sc)
				a.Or32(acc, part)
				for _, r := range []x86.Reg{mask0f, tmp, sc, p3, p2, p1, in} {
					ctx.ReleaseVector(r)
				}
			}

			ctx.ReleaseGP(src)
			ctx.ReleaseGP(part)
			return ctx.OutputI32(acc)
		},
	}
}

package wide

import (
	wago "github.com/wago-org/wago"
	x86 "github.com/wago-org/wago/codegen/amd64"
)

func amd64ChunkBytes(opcode uint32) uint32 {
	switch opcode {
	case 98, 124, 125, 126, 127, 232, 233, 244, 245, 248, 249, 250, 251, 257, 258, 274, 275:
		return 16
	default:
		return 32
	}
}

func emitAMD64YMM(ctx wago.AMD64LoweringContext, opcode uint32, raw []uint8) (x86.Reg, error) {
	inputs := make([]x86.Reg, len(raw))
	for i := range raw {
		inputs[i] = x86.Reg(raw[i])
	}
	a := ctx.Encoder()
	dst := inputs[0]
	binary := func(op func(x86.Reg, x86.Reg, x86.Reg)) { op(dst, dst, inputs[1]) }
	unary := func(op func(x86.Reg, x86.Reg)) { op(dst, dst) }
	allOnes := func(exclude ...x86.Reg) x86.Reg {
		m := ctx.AllocYMM(exclude...)
		a.YPcmpeqb(m, m, m)
		return m
	}
	invert := func() {
		m := allOnes(dst)
		a.YPxor(dst, dst, m)
		ctx.ReleaseVector(m)
	}
	signedCmp := func(op func(x86.Reg, x86.Reg, x86.Reg), swap, inv bool) {
		left, right := dst, inputs[1]
		if swap {
			left, right = right, left
		}
		op(dst, left, right)
		if inv {
			invert()
		}
	}
	unsignedCmp := func(op func(x86.Reg, x86.Reg, x86.Reg), lo, hi uint64, swap, inv bool) {
		bias := ctx.ConstYMMRepeated128(lo, hi)
		a.YPxor(dst, dst, bias)
		a.YPxor(inputs[1], inputs[1], bias)
		ctx.ReleaseVector(bias)
		signedCmp(op, swap, inv)
	}
	integerNeg := func(op func(x86.Reg, x86.Reg, x86.Reg)) {
		z := ctx.AllocYMM(dst)
		a.YPxor(z, z, z)
		op(dst, z, dst)
		ctx.ReleaseVector(z)
	}
	v128FloatMinMax := func(f64, isMax bool) {
		tmp := ctx.AllocYMM(dst, inputs[1])
		cmp := ctx.AllocYMM(dst, inputs[1], tmp)
		packed := a.VFPackedMin
		if isMax {
			packed = a.VFPackedMax
		}
		packed(tmp, dst, inputs[1], f64)
		packed(dst, inputs[1], dst, f64)
		a.VFCmpPacked(cmp, tmp, dst, f64, 0x03)
		pp := byte(0)
		if f64 {
			pp = 1
		}
		if isMax {
			a.VSseRRR(pp, 0x54, tmp, tmp, dst)
		} else {
			a.VSseRRR(pp, 0x56, tmp, tmp, dst)
		}
		a.VSseRRR(pp, 0x56, tmp, tmp, cmp)
		if f64 {
			a.VPsrlqImm(cmp, cmp, 13)
		} else {
			a.VPsrldImm(cmp, cmp, 10)
		}
		a.VSseRRR(pp, 0x55, tmp, cmp, tmp)
		ctx.ReleaseVector(cmp)
		ctx.ReleaseVector(dst)
		dst = tmp
	}
	v128TruncSat := func(signed bool) {
		tmp := ctx.AllocYMM(dst)
		if signed {
			a.VMovdqu(tmp, dst)
			a.VFCmpPacked(tmp, tmp, tmp, false, 0x00)
			a.VSseRRR(0, 0x54, dst, dst, tmp)
			a.VSseRRR(0, 0x57, tmp, tmp, dst)
			a.Vcvttps2dq(dst, dst)
			a.VSseRRR(0, 0x54, tmp, tmp, dst)
			a.VPsradImm(tmp, tmp, 31)
			a.VPxor(dst, dst, tmp)
		} else {
			zero := ctx.AllocYMM(dst, tmp)
			tmp2 := ctx.AllocYMM(dst, tmp, zero)
			a.VPxor(zero, zero, zero)
			a.VSseRRR(0, 0x5f, dst, dst, zero)
			a.VPcmpeqd(tmp, tmp, tmp)
			a.VPsrldImm(tmp, tmp, 1)
			a.Vcvtdq2ps(tmp, tmp)
			a.VMovdqu(tmp2, dst)
			a.Vcvttps2dq(dst, dst)
			a.VSseRRR(0, 0x5c, tmp2, tmp2, tmp)
			a.VFCmpPacked(tmp, tmp, tmp2, false, 0x12)
			a.Vcvttps2dq(tmp2, tmp2)
			a.VPxor(tmp2, tmp2, tmp)
			a.VPxor(tmp, tmp, tmp)
			a.VPmaxsd(tmp2, tmp2, tmp)
			a.VPaddd(dst, dst, tmp2)
			ctx.ReleaseVector(tmp2)
			ctx.ReleaseVector(zero)
		}
		ctx.ReleaseVector(tmp)
	}
	switch opcode {
	case 35:
		binary(a.YPcmpeqb)
	case 36:
		binary(a.YPcmpeqb)
		invert()
	case 37:
		signedCmp(a.YPcmpgtb, true, false)
	case 38:
		unsignedCmp(a.YPcmpgtb, 0x8080808080808080, 0x8080808080808080, true, false)
	case 39:
		binary(a.YPcmpgtb)
	case 40:
		unsignedCmp(a.YPcmpgtb, 0x8080808080808080, 0x8080808080808080, false, false)
	case 41:
		signedCmp(a.YPcmpgtb, false, true)
	case 42:
		unsignedCmp(a.YPcmpgtb, 0x8080808080808080, 0x8080808080808080, false, true)
	case 43:
		signedCmp(a.YPcmpgtb, true, true)
	case 44:
		unsignedCmp(a.YPcmpgtb, 0x8080808080808080, 0x8080808080808080, true, true)
	case 45:
		binary(a.YPcmpeqw)
	case 46:
		binary(a.YPcmpeqw)
		invert()
	case 47:
		signedCmp(a.YPcmpgtw, true, false)
	case 48:
		unsignedCmp(a.YPcmpgtw, 0x8000800080008000, 0x8000800080008000, true, false)
	case 49:
		binary(a.YPcmpgtw)
	case 50:
		unsignedCmp(a.YPcmpgtw, 0x8000800080008000, 0x8000800080008000, false, false)
	case 51:
		signedCmp(a.YPcmpgtw, false, true)
	case 52:
		unsignedCmp(a.YPcmpgtw, 0x8000800080008000, 0x8000800080008000, false, true)
	case 53:
		signedCmp(a.YPcmpgtw, true, true)
	case 54:
		unsignedCmp(a.YPcmpgtw, 0x8000800080008000, 0x8000800080008000, true, true)
	case 55:
		binary(a.YPcmpeqd)
	case 56:
		binary(a.YPcmpeqd)
		invert()
	case 57:
		signedCmp(a.YPcmpgtd, true, false)
	case 58:
		unsignedCmp(a.YPcmpgtd, 0x8000000080000000, 0x8000000080000000, true, false)
	case 59:
		binary(a.YPcmpgtd)
	case 60:
		unsignedCmp(a.YPcmpgtd, 0x8000000080000000, 0x8000000080000000, false, false)
	case 61:
		signedCmp(a.YPcmpgtd, false, true)
	case 62:
		unsignedCmp(a.YPcmpgtd, 0x8000000080000000, 0x8000000080000000, false, true)
	case 63:
		signedCmp(a.YPcmpgtd, true, true)
	case 64:
		unsignedCmp(a.YPcmpgtd, 0x8000000080000000, 0x8000000080000000, true, true)
	case 65, 66, 67, 68, 69, 70:
		pred := [...]byte{0x00, 0x04, 0x11, 0x1e, 0x12, 0x1d}[opcode-65]
		a.YFCmpPacked(dst, dst, inputs[1], false, pred)
	case 71, 72, 73, 74, 75, 76:
		pred := [...]byte{0x00, 0x04, 0x11, 0x1e, 0x12, 0x1d}[opcode-71]
		a.YFCmpPacked(dst, dst, inputs[1], true, pred)
	case 77:
		invert()
	case 78:
		binary(a.YPand)
	case 79:
		m := allOnes(dst, inputs[1])
		a.YPxor(inputs[1], inputs[1], m)
		ctx.ReleaseVector(m)
		binary(a.YPand)
	case 80:
		binary(a.YPor)
	case 81:
		binary(a.YPxor)
	case 82, 265, 266, 267, 268:
		a.YPand(dst, dst, inputs[2])
		a.YPandn(inputs[1], inputs[2], inputs[1])
		a.YPor(dst, dst, inputs[1])
	case 96:
		unary(a.YPabsb)
	case 97:
		integerNeg(a.YPsubb)
	case 98:
		hi := ctx.AllocYMM(dst)
		a.VPsrlwImm(hi, dst, 4)
		mask := ctx.ConstYMMRepeated128(0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f)
		lut := ctx.ConstYMMRepeated128(0x0302020102010100, 0x0403030203020201)
		a.VPand(dst, dst, mask)
		a.VPand(hi, hi, mask)
		a.VPshufb(dst, lut, dst)
		a.VPshufb(hi, lut, hi)
		a.VPaddb(dst, dst, hi)
		ctx.ReleaseVector(lut)
		ctx.ReleaseVector(mask)
		ctx.ReleaseVector(hi)
	case 103, 104, 105, 106:
		mode := [...]byte{0x02, 0x01, 0x03, 0x00}[opcode-103]
		a.YFRoundPacked(dst, dst, false, mode)
	case 110:
		binary(a.YPaddb)
	case 111:
		binary(a.YPaddsb)
	case 112:
		binary(a.YPaddusb)
	case 113:
		binary(a.YPsubb)
	case 114:
		binary(a.YPsubsb)
	case 115:
		binary(a.YPsubusb)
	case 116, 117, 122, 148:
		modes := map[uint32]byte{116: 0x02, 117: 0x01, 122: 0x03, 148: 0x00}
		a.YFRoundPacked(dst, dst, true, modes[opcode])
	case 118:
		binary(a.YPminsb)
	case 119:
		binary(a.YPminub)
	case 120:
		binary(a.YPmaxsb)
	case 121:
		binary(a.YPmaxub)
	case 123:
		binary(a.YPavgb)
	case 124, 125:
		ones := ctx.AllocYMM(dst)
		a.VPcmpeqb(ones, ones, ones)
		a.VPabsb(ones, ones)
		if opcode == 124 {
			a.VPmaddubsw(dst, ones, dst)
		} else {
			a.VPmaddubsw(dst, dst, ones)
		}
		ctx.ReleaseVector(ones)
	case 126:
		ones := ctx.AllocYMM(dst)
		a.VPcmpeqw(ones, ones, ones)
		a.VPsrlwImm(ones, ones, 15)
		a.VPmaddwd(dst, dst, ones)
		ctx.ReleaseVector(ones)
	case 127:
		high := ctx.AllocYMM(dst)
		zero := ctx.AllocYMM(dst, high)
		a.VPor(high, dst, dst)
		a.VPxor(zero, zero, zero)
		a.VPunpcklwd(dst, dst, zero)
		a.VPunpckhwd(high, high, zero)
		a.VPhaddd(dst, dst, high)
		ctx.ReleaseVector(zero)
		ctx.ReleaseVector(high)
	case 128:
		unary(a.YPabsw)
	case 129:
		integerNeg(a.YPsubw)
	case 130, 273:
		binary(a.YPmulhrsw)
	case 142:
		binary(a.YPaddw)
	case 143:
		binary(a.YPaddsw)
	case 144:
		binary(a.YPaddusw)
	case 145:
		binary(a.YPsubw)
	case 146:
		binary(a.YPsubsw)
	case 147:
		binary(a.YPsubusw)
	case 149:
		binary(a.YPmullw)
	case 150:
		binary(a.YPminsw)
	case 151:
		binary(a.YPminuw)
	case 152:
		binary(a.YPmaxsw)
	case 153:
		binary(a.YPmaxuw)
	case 155:
		binary(a.YPavgw)
	case 160:
		unary(a.YPabsd)
	case 161:
		integerNeg(a.YPsubd)
	case 174:
		binary(a.YPaddd)
	case 177:
		binary(a.YPsubd)
	case 181:
		binary(a.YPmulld)
	case 182:
		binary(a.YPminsd)
	case 183:
		binary(a.YPminud)
	case 184:
		binary(a.YPmaxsd)
	case 185:
		binary(a.YPmaxud)
	case 186:
		binary(a.YPmaddwd)
	case 192:
		sign := ctx.AllocYMM(dst)
		a.YPxor(sign, sign, sign)
		a.YPcmpgtq(sign, sign, dst)
		a.YPxor(dst, dst, sign)
		a.YPsubq(dst, dst, sign)
		ctx.ReleaseVector(sign)
	case 193:
		integerNeg(a.YPsubq)
	case 206:
		binary(a.YPaddq)
	case 209:
		binary(a.YPsubq)
	case 213:
		cross := ctx.AllocYMM(dst, inputs[1])
		tmp := ctx.AllocYMM(dst, inputs[1], cross)
		a.YPsrlqImm(cross, inputs[1], 32)
		a.YPmuludq(cross, cross, dst)
		a.YPsrlqImm(tmp, dst, 32)
		a.YPmuludq(tmp, tmp, inputs[1])
		a.YPaddq(cross, cross, tmp)
		a.YPsllqImm(cross, cross, 32)
		a.YPmuludq(dst, dst, inputs[1])
		a.YPaddq(dst, dst, cross)
		ctx.ReleaseVector(tmp)
		ctx.ReleaseVector(cross)
	case 214:
		binary(a.YPcmpeqq)
	case 215:
		binary(a.YPcmpeqq)
		invert()
	case 216:
		signedCmp(a.YPcmpgtq, true, false)
	case 217:
		binary(a.YPcmpgtq)
	case 218:
		signedCmp(a.YPcmpgtq, false, true)
	case 219:
		signedCmp(a.YPcmpgtq, true, true)
	case 224:
		mask := ctx.ConstYMMRepeated128(0x7fffffff7fffffff, 0x7fffffff7fffffff)
		a.YSseRRR(0, 0x54, dst, dst, mask)
		ctx.ReleaseVector(mask)
	case 225:
		mask := ctx.ConstYMMRepeated128(0x8000000080000000, 0x8000000080000000)
		a.YSseRRR(0, 0x57, dst, dst, mask)
		ctx.ReleaseVector(mask)
	case 227:
		a.YFPackedSqrt(dst, dst, false)
	case 228:
		binary(func(d, x, y x86.Reg) { a.YFPackedAdd(d, x, y, false) })
	case 229:
		binary(func(d, x, y x86.Reg) { a.YFPackedSub(d, x, y, false) })
	case 230:
		binary(func(d, x, y x86.Reg) { a.YFPackedMul(d, x, y, false) })
	case 231:
		binary(func(d, x, y x86.Reg) { a.YFPackedDiv(d, x, y, false) })
	case 232:
		v128FloatMinMax(false, false)
	case 233:
		v128FloatMinMax(false, true)
	case 234:
		a.YFPackedMin(dst, inputs[1], dst, false)
	case 235:
		a.YFPackedMax(dst, inputs[1], dst, false)
	case 236:
		mask := ctx.ConstYMMRepeated128(0x7fffffffffffffff, 0x7fffffffffffffff)
		a.YSseRRR(1, 0x54, dst, dst, mask)
		ctx.ReleaseVector(mask)
	case 237:
		mask := ctx.ConstYMMRepeated128(0x8000000000000000, 0x8000000000000000)
		a.YSseRRR(1, 0x57, dst, dst, mask)
		ctx.ReleaseVector(mask)
	case 239:
		a.YFPackedSqrt(dst, dst, true)
	case 240:
		binary(func(d, x, y x86.Reg) { a.YFPackedAdd(d, x, y, true) })
	case 241:
		binary(func(d, x, y x86.Reg) { a.YFPackedSub(d, x, y, true) })
	case 242:
		binary(func(d, x, y x86.Reg) { a.YFPackedMul(d, x, y, true) })
	case 243:
		binary(func(d, x, y x86.Reg) { a.YFPackedDiv(d, x, y, true) })
	case 244:
		v128FloatMinMax(true, false)
	case 245:
		v128FloatMinMax(true, true)
	case 246:
		a.YFPackedMin(dst, inputs[1], dst, true)
	case 247:
		a.YFPackedMax(dst, inputs[1], dst, true)
	case 248, 257:
		v128TruncSat(true)
	case 249, 258:
		v128TruncSat(false)
	case 250:
		a.Vcvtdq2ps(dst, dst)
	case 251:
		mask := ctx.ConstYMMRepeated128(0x0000ffff0000ffff, 0x0000ffff0000ffff)
		low := ctx.AllocYMM(dst, mask)
		high := ctx.AllocYMM(dst, mask, low)
		a.VPand(low, dst, mask)
		a.VPsrldImm(high, dst, 16)
		a.Vcvtdq2ps(low, low)
		a.Vcvtdq2ps(high, high)
		scale := ctx.ConstYMMRepeated128(0x4780000047800000, 0x4780000047800000)
		a.VFPackedMul(high, high, scale, false)
		a.VFPackedAdd(low, low, high, false)
		ctx.ReleaseVector(scale)
		ctx.ReleaseVector(high)
		ctx.ReleaseVector(mask)
		ctx.ReleaseVector(dst)
		dst = low
	case 261, 263:
		a.YFPackedMul(dst, dst, inputs[1], opcode == 263)
		a.YFPackedAdd(dst, dst, inputs[2], opcode == 263)
	case 262, 264:
		a.YFPackedMul(dst, dst, inputs[1], opcode == 264)
		a.YFPackedSub(inputs[2], inputs[2], dst, opcode == 264)
		ctx.ReleaseVector(dst)
		dst = inputs[2]
		inputs[2] = inputs[0]
	case 269:
		binary(func(d, x, y x86.Reg) { a.YFPackedMin(d, x, y, false) })
	case 270:
		binary(func(d, x, y x86.Reg) { a.YFPackedMax(d, x, y, false) })
	case 271:
		binary(func(d, x, y x86.Reg) { a.YFPackedMin(d, x, y, true) })
	case 272:
		binary(func(d, x, y x86.Reg) { a.YFPackedMax(d, x, y, true) })
	case 274, 275:
		sign := ctx.AllocYMM(dst, inputs[1])
		abs := ctx.AllocYMM(dst, inputs[1], sign)
		a.VPxor(sign, sign, sign)
		a.VPcmpgtb(sign, sign, dst)
		a.VPxor(inputs[1], inputs[1], sign)
		a.VPsubb(inputs[1], inputs[1], sign)
		a.VPabsb(abs, dst)
		a.VPmaddubsw(dst, abs, inputs[1])
		ctx.ReleaseVector(abs)
		ctx.ReleaseVector(sign)
		if opcode == 275 {
			ones := ctx.AllocYMM(dst, inputs[2])
			a.VPcmpeqw(ones, ones, ones)
			a.VPsrlwImm(ones, ones, 15)
			a.VPmaddwd(dst, dst, ones)
			a.VPaddd(dst, dst, inputs[2])
			ctx.ReleaseVector(ones)
		}
	default:
		for _, r := range inputs {
			ctx.ReleaseVector(r)
		}
		return 0, unsupportedTarget("amd64", opcode)
	}
	for _, r := range inputs[1:] {
		ctx.ReleaseVector(r)
	}
	return dst, nil
}

package wide

import (
	wago "github.com/wago-org/wago"
	a64 "github.com/wago-org/wago/codegen/arm64"
)

// emitARM64NEON implements one 128-bit slice of a wide semantic instruction.
// Width decomposition is handled by arm64Lowering; this function deliberately
// owns every semantic choice so Wago remains a raw custom-instruction backend.
func emitARM64NEON(ctx wago.ARM64LoweringContext, opcode uint32, raw []uint8) (a64.Reg, error) {
	inputs := make([]a64.Reg, len(raw))
	for i := range raw {
		inputs[i] = a64.Reg(raw[i])
	}
	a := ctx.Encoder()
	dst := inputs[0]
	binary := func(op func(a64.Reg, a64.Reg, a64.Reg)) { op(dst, dst, inputs[1]) }
	unary := func(op func(a64.Reg, a64.Reg)) { op(dst, dst) }
	invert := func() { a.NeonNot16b(dst, dst) }
	cmp := func(op func(a64.Reg, a64.Reg, a64.Reg), swap, inv bool) {
		left, right := dst, inputs[1]
		if swap {
			left, right = right, left
		}
		op(dst, left, right)
		if inv {
			invert()
		}
	}
	fcmp := func(f64 bool, pred byte, inv bool) {
		a.NeonFcmp(dst, dst, inputs[1], f64, pred)
		if inv {
			invert()
		}
	}
	pminmax := func(f64, max bool) {
		mask := ctx.AllocVector(inputs...)
		pred := byte(0x11)
		if max {
			pred = 0x1e
		}
		// Pseudo-min/max chooses b only when b is strictly smaller/larger.
		// Ordered comparison leaves a winning equal, signed-zero, and NaN lanes.
		a.NeonFcmp(mask, inputs[1], inputs[0], f64, pred)
		a.NeonBsl16b(mask, inputs[1], inputs[0])
		dst = mask
	}
	i64mul := func() {
		t := ctx.AllocVector(inputs...)
		a.NeonRev64S(t, inputs[1])
		a.NeonMulS(t, t, inputs[0])
		a.NeonUaddlpDfromS(t, t)
		a.NeonShlD(t, t, 32)

		aLo := ctx.AllocVector(append(inputs, t)...)
		bLo := ctx.AllocVector(append(inputs, t, aLo)...)
		a.NeonXtnSfromD(aLo, inputs[0])
		a.NeonXtnSfromD(bLo, inputs[1])
		lo := ctx.AllocVector(append(inputs, t, aLo, bLo)...)
		a.NeonUmullDfromS(lo, aLo, bLo)
		a.NeonAddD(dst, t, lo)
		ctx.ReleaseVector(lo)
		ctx.ReleaseVector(bLo)
		ctx.ReleaseVector(aLo)
		ctx.ReleaseVector(t)
	}
	dotI16 := func() {
		lo := ctx.AllocVector(inputs...)
		hi := ctx.AllocVector(append(inputs, lo)...)
		a.NeonSmullSfromH(lo, inputs[0], inputs[1])
		a.NeonSmull2SfromH(hi, inputs[0], inputs[1])
		a.NeonAddpS(dst, lo, hi)
		ctx.ReleaseVector(hi)
		ctx.ReleaseVector(lo)
	}
	relaxedDot := func(add bool) {
		lo := ctx.AllocVector(inputs...)
		hi := ctx.AllocVector(append(inputs, lo)...)
		a.NeonSmullHfromB(lo, inputs[0], inputs[1])
		a.NeonSmull2HfromB(hi, inputs[0], inputs[1])
		a.NeonSaddlpSfromH(lo, lo)
		a.NeonSaddlpSfromH(hi, hi)
		a.NeonSqxtnHfromS(dst, lo)
		a.NeonSqxtn2HfromS(dst, hi)
		if add {
			a.NeonSaddlpSfromH(dst, dst)
			a.NeonAddS(dst, dst, inputs[2])
		}
		ctx.ReleaseVector(hi)
		ctx.ReleaseVector(lo)
	}

	switch opcode {
	case 35:
		binary(a.NeonCmeqB)
	case 36:
		binary(a.NeonCmeqB)
		invert()
	case 37:
		cmp(a.NeonCmgtB, true, false)
	case 38:
		cmp(a.NeonCmhiB, true, false)
	case 39:
		binary(a.NeonCmgtB)
	case 40:
		binary(a.NeonCmhiB)
	case 41:
		cmp(a.NeonCmgeB, true, false)
	case 42:
		cmp(a.NeonCmhsB, true, false)
	case 43:
		binary(a.NeonCmgeB)
	case 44:
		binary(a.NeonCmhsB)
	case 45:
		binary(a.NeonCmeqH)
	case 46:
		binary(a.NeonCmeqH)
		invert()
	case 47:
		cmp(a.NeonCmgtH, true, false)
	case 48:
		cmp(a.NeonCmhiH, true, false)
	case 49:
		binary(a.NeonCmgtH)
	case 50:
		binary(a.NeonCmhiH)
	case 51:
		cmp(a.NeonCmgeH, true, false)
	case 52:
		cmp(a.NeonCmhsH, true, false)
	case 53:
		binary(a.NeonCmgeH)
	case 54:
		binary(a.NeonCmhsH)
	case 55:
		binary(a.NeonCmeqS)
	case 56:
		binary(a.NeonCmeqS)
		invert()
	case 57:
		cmp(a.NeonCmgtS, true, false)
	case 58:
		cmp(a.NeonCmhiS, true, false)
	case 59:
		binary(a.NeonCmgtS)
	case 60:
		binary(a.NeonCmhiS)
	case 61:
		cmp(a.NeonCmgeS, true, false)
	case 62:
		cmp(a.NeonCmhsS, true, false)
	case 63:
		binary(a.NeonCmgeS)
	case 64:
		binary(a.NeonCmhsS)
	case 65:
		fcmp(false, 0x00, false)
	case 66:
		fcmp(false, 0x00, true)
	case 67:
		fcmp(false, 0x11, false)
	case 68:
		fcmp(false, 0x1e, false)
	case 69:
		fcmp(false, 0x12, false)
	case 70:
		fcmp(false, 0x1d, false)
	case 71:
		fcmp(true, 0x00, false)
	case 72:
		fcmp(true, 0x00, true)
	case 73:
		fcmp(true, 0x11, false)
	case 74:
		fcmp(true, 0x1e, false)
	case 75:
		fcmp(true, 0x12, false)
	case 76:
		fcmp(true, 0x1d, false)
	case 77:
		unary(a.NeonNot16b)
	case 78:
		binary(a.NeonAnd16b)
	case 79:
		binary(a.NeonAndn16b)
	case 80:
		binary(a.NeonOrr16b)
	case 81:
		binary(a.NeonEor16b)
	case 82, 265, 266, 267, 268:
		// BSL uses its destination as the mask.
		a.NeonBsl16b(inputs[2], inputs[0], inputs[1])
		dst = inputs[2]
	case 96:
		unary(a.NeonAbsB)
	case 97:
		unary(a.NeonNegB)
	case 98:
		unary(a.NeonCntB)
	case 103, 104, 105, 106:
		mode := [...]byte{'p', 'm', 'z', 'n'}[opcode-103]
		a.NeonFrint(dst, dst, false, mode)
	case 110:
		binary(a.NeonAddB)
	case 111:
		binary(a.NeonSqaddB)
	case 112:
		binary(a.NeonUqaddB)
	case 113:
		binary(a.NeonSubB)
	case 114:
		binary(a.NeonSqsubB)
	case 115:
		binary(a.NeonUqsubB)
	case 116:
		a.NeonFrint(dst, dst, true, 'p')
	case 117:
		a.NeonFrint(dst, dst, true, 'm')
	case 118:
		binary(a.NeonSminB)
	case 119:
		binary(a.NeonUminB)
	case 120:
		binary(a.NeonSmaxB)
	case 121:
		binary(a.NeonUmaxB)
	case 122:
		a.NeonFrint(dst, dst, true, 'z')
	case 123:
		binary(a.NeonUrhaddB)
	case 124:
		unary(a.NeonSaddlpHfromB)
	case 125:
		unary(a.NeonUaddlpHfromB)
	case 126:
		unary(a.NeonSaddlpSfromH)
	case 127:
		unary(a.NeonUaddlpSfromH)
	case 128:
		unary(a.NeonAbsH)
	case 129:
		unary(a.NeonNegH)
	case 130, 273:
		binary(a.NeonSqrdmulhH)
	case 142:
		binary(a.NeonAddH)
	case 143:
		binary(a.NeonSqaddH)
	case 144:
		binary(a.NeonUqaddH)
	case 145:
		binary(a.NeonSubH)
	case 146:
		binary(a.NeonSqsubH)
	case 147:
		binary(a.NeonUqsubH)
	case 148:
		a.NeonFrint(dst, dst, true, 'n')
	case 149:
		binary(a.NeonMulH)
	case 150:
		binary(a.NeonSminH)
	case 151:
		binary(a.NeonUminH)
	case 152:
		binary(a.NeonSmaxH)
	case 153:
		binary(a.NeonUmaxH)
	case 155:
		binary(a.NeonUrhaddH)
	case 160:
		unary(a.NeonAbsS)
	case 161:
		unary(a.NeonNegS)
	case 174:
		binary(a.NeonAddS)
	case 177:
		binary(a.NeonSubS)
	case 181:
		binary(a.NeonMulS)
	case 182:
		binary(a.NeonSminS)
	case 183:
		binary(a.NeonUminS)
	case 184:
		binary(a.NeonSmaxS)
	case 185:
		binary(a.NeonUmaxS)
	case 186:
		dotI16()
	case 192:
		unary(a.NeonAbsD)
	case 193:
		unary(a.NeonNegD)
	case 206:
		binary(a.NeonAddD)
	case 209:
		binary(a.NeonSubD)
	case 213:
		i64mul()
	case 214:
		binary(a.NeonCmeqD)
	case 215:
		binary(a.NeonCmeqD)
		invert()
	case 216:
		cmp(a.NeonCmgtD, true, false)
	case 217:
		binary(a.NeonCmgtD)
	case 218:
		cmp(a.NeonCmgeD, true, false)
	case 219:
		binary(a.NeonCmgeD)
	case 224:
		a.NeonFabs(dst, dst, false)
	case 225:
		a.NeonFneg(dst, dst, false)
	case 227:
		a.NeonFsqrt(dst, dst, false)
	case 228:
		a.NeonFadd(dst, dst, inputs[1], false)
	case 229:
		a.NeonFsub(dst, dst, inputs[1], false)
	case 230:
		a.NeonFmul(dst, dst, inputs[1], false)
	case 231:
		a.NeonFdiv(dst, dst, inputs[1], false)
	case 232:
		a.NeonFmin(dst, dst, inputs[1], false)
	case 233:
		a.NeonFmax(dst, dst, inputs[1], false)
	case 234:
		pminmax(false, false)
	case 235:
		pminmax(false, true)
	case 236:
		a.NeonFabs(dst, dst, true)
	case 237:
		a.NeonFneg(dst, dst, true)
	case 239:
		a.NeonFsqrt(dst, dst, true)
	case 240:
		a.NeonFadd(dst, dst, inputs[1], true)
	case 241:
		a.NeonFsub(dst, dst, inputs[1], true)
	case 242:
		a.NeonFmul(dst, dst, inputs[1], true)
	case 243:
		a.NeonFdiv(dst, dst, inputs[1], true)
	case 244:
		a.NeonFmin(dst, dst, inputs[1], true)
	case 245:
		a.NeonFmax(dst, dst, inputs[1], true)
	case 246:
		pminmax(true, false)
	case 247:
		pminmax(true, true)
	case 248, 257:
		a.NeonFcvtzsSfromS(dst, dst)
	case 249, 258:
		a.NeonFcvtzuSfromS(dst, dst)
	case 250:
		a.NeonScvtfSfromS(dst, dst)
	case 251:
		a.NeonUcvtfSfromS(dst, dst)
	case 261, 262:
		a.NeonFmul(dst, dst, inputs[1], false)
		if opcode == 261 {
			a.NeonFadd(dst, dst, inputs[2], false)
		} else {
			a.NeonFsub(dst, inputs[2], dst, false)
		}
	case 263, 264:
		a.NeonFmul(dst, dst, inputs[1], true)
		if opcode == 263 {
			a.NeonFadd(dst, dst, inputs[2], true)
		} else {
			a.NeonFsub(dst, inputs[2], dst, true)
		}
	case 269:
		a.NeonFmin(dst, dst, inputs[1], false)
	case 270:
		a.NeonFmax(dst, dst, inputs[1], false)
	case 271:
		a.NeonFmin(dst, dst, inputs[1], true)
	case 272:
		a.NeonFmax(dst, dst, inputs[1], true)
	case 274:
		relaxedDot(false)
	case 275:
		relaxedDot(true)
	default:
		for _, r := range inputs {
			ctx.ReleaseVector(r)
		}
		return 0, unsupportedTarget("arm64", opcode)
	}
	for _, r := range inputs {
		if r != dst {
			ctx.ReleaseVector(r)
		}
	}
	return dst, nil
}

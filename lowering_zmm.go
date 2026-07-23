package wide

import (
	"os"
	"strings"

	wago "github.com/wago-org/wago"
	x86 "github.com/wago-org/wago/codegen/amd64"
)

type zmmEncoding struct {
	opcodeMap, pp, op byte
	w, unary, swap    bool
}

func shouldUseZMM(opcode uint32) bool {
	if wideEnvEnabled("DISABLE_AVX512") {
		return false
	}
	if _, ok := zmmEncodingFor(opcode); !ok && opcode != 82 && !(opcode >= 265 && opcode <= 268) {
		return false
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	flags := string(data)
	if !strings.Contains(flags, " avx512f ") || !strings.Contains(flags, " avx512dq ") || !strings.Contains(flags, " avx512bw ") {
		return false
	}
	return wideEnvEnabled("FORCE_AVX512") || opcode == 82 || opcode == 192 || opcode == 213 || opcode >= 265 && opcode <= 268
}

func wideEnvEnabled(name string) bool {
	return os.Getenv("WIDE_"+name) == "1" || os.Getenv("WAGO_"+name) == "1"
}

func zmmEncodingFor(opcode uint32) (zmmEncoding, bool) {
	e := zmmEncoding{opcodeMap: 1, pp: 1}
	switch opcode {
	case 78:
		e.op = 0xdb
	case 80:
		e.op = 0xeb
	case 81:
		e.op = 0xef
	case 96:
		e.opcodeMap, e.op, e.unary = 2, 0x1c, true
	case 110:
		e.op = 0xfc
	case 111:
		e.op = 0xec
	case 112:
		e.op = 0xdc
	case 113:
		e.op = 0xf8
	case 114:
		e.op = 0xe8
	case 115:
		e.op = 0xd8
	case 118:
		e.opcodeMap, e.op = 2, 0x38
	case 119:
		e.op = 0xda
	case 120:
		e.opcodeMap, e.op = 2, 0x3c
	case 121:
		e.op = 0xde
	case 123:
		e.op = 0xe0
	case 128:
		e.opcodeMap, e.op, e.unary = 2, 0x1d, true
	case 130, 273:
		e.opcodeMap, e.op = 2, 0x0b
	case 142:
		e.op = 0xfd
	case 143:
		e.op = 0xed
	case 144:
		e.op = 0xdd
	case 145:
		e.op = 0xf9
	case 146:
		e.op = 0xe9
	case 147:
		e.op = 0xd9
	case 149:
		e.op = 0xd5
	case 150:
		e.op = 0xea
	case 151:
		e.opcodeMap, e.op = 2, 0x3a
	case 152:
		e.op = 0xee
	case 153:
		e.opcodeMap, e.op = 2, 0x3e
	case 155:
		e.op = 0xe3
	case 160:
		e.opcodeMap, e.op, e.unary = 2, 0x1e, true
	case 174:
		e.op = 0xfe
	case 177:
		e.op = 0xfa
	case 181:
		e.opcodeMap, e.op = 2, 0x40
	case 182:
		e.opcodeMap, e.op = 2, 0x39
	case 183:
		e.opcodeMap, e.op = 2, 0x3b
	case 184:
		e.opcodeMap, e.op = 2, 0x3d
	case 185:
		e.opcodeMap, e.op = 2, 0x3f
	case 186:
		e.op = 0xf5
	case 192:
		e.opcodeMap, e.op, e.w, e.unary = 2, 0x1f, true, true
	case 206:
		e.op, e.w = 0xd4, true
	case 209:
		e.op, e.w = 0xfb, true
	case 213:
		e.opcodeMap, e.op, e.w = 2, 0x40, true
	case 227:
		e.pp, e.op, e.unary = 0, 0x51, true
	case 228:
		e.pp, e.op = 0, 0x58
	case 229:
		e.pp, e.op = 0, 0x5c
	case 230:
		e.pp, e.op = 0, 0x59
	case 231:
		e.pp, e.op = 0, 0x5e
	case 234:
		e.pp, e.op, e.swap = 0, 0x5d, true
	case 235:
		e.pp, e.op, e.swap = 0, 0x5f, true
	case 239:
		e.op, e.w, e.unary = 0x51, true, true
	case 240:
		e.op, e.w = 0x58, true
	case 241:
		e.op, e.w = 0x5c, true
	case 242:
		e.op, e.w = 0x59, true
	case 243:
		e.op, e.w = 0x5e, true
	case 246:
		e.op, e.w, e.swap = 0x5d, true, true
	case 247:
		e.op, e.w, e.swap = 0x5f, true, true
	case 269:
		e.pp, e.op = 0, 0x5d
	case 270:
		e.pp, e.op = 0, 0x5f
	case 271:
		e.op, e.w = 0x5d, true
	case 272:
		e.op, e.w = 0x5f, true
	default:
		return zmmEncoding{}, false
	}
	return e, true
}

func emitAMD64ZMM(ctx wago.AMD64LoweringContext, opcode uint32, raw []uint8) (x86.Reg, error) {
	inputs := make([]x86.Reg, len(raw))
	for i := range raw {
		inputs[i] = x86.Reg(raw[i])
	}
	dst := inputs[0]
	if opcode == 82 || opcode >= 265 && opcode <= 268 {
		ctx.Encoder().ZPternlogd(dst, inputs[1], inputs[2], 0xe4)
	} else {
		e, ok := zmmEncodingFor(opcode)
		if !ok {
			return 0, unsupportedTarget("amd64-zmm", opcode)
		}
		if e.unary {
			ctx.Encoder().ZSIMDRR(e.opcodeMap, e.pp, e.op, e.w, dst, dst)
		} else {
			left, right := dst, inputs[1]
			if e.swap {
				left, right = right, left
			}
			ctx.Encoder().ZSIMDRRR(e.opcodeMap, e.pp, e.op, e.w, dst, left, right)
		}
	}
	for _, reg := range inputs[1:] {
		ctx.Release(reg)
	}
	return dst, nil
}

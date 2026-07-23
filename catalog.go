package wide

import "strings"

// shape is the engine-owned operation shape used to derive pointer arity. The
// direct-import plugin needs no fallback-body encoder or metadata codec.
type shape byte

const (
	shapeUnary shape = iota + 1
	shapeBinary
	shapeTernary
	shapeShift
	shapeSplatI32
	shapeSplatI64
	shapeSplatF32
	shapeSplatF64
	shapeNarrow
	shapeReduceAny
	shapeReduceAll
	shapeReduceMask
	shapeCrossUnaryLow
	shapeCrossUnaryHigh
	shapeCrossBinaryLow
	shapeCrossBinaryHigh
	shapePackLow64
	shapeLow64Windows
	shapeSwizzle
	shapeLoad
	shapeStore
	shapeMemLoadLane
	shapeMemStoreLane
	shapeConst
	shapeShuffle
	shapeExtractI32
	shapeExtractI64
	shapeExtractF32
	shapeExtractF64
	shapeReplaceI32
	shapeReplaceI64
	shapeReplaceF32
	shapeReplaceF64
)

type operation struct {
	Subopcode uint32
	Shape     shape
}

func operationFor(sub uint32) (operation, bool) {
	sh, ok := shapes[sub]
	return operation{Subopcode: sub, Shape: sh}, ok
}

var shapes = func() map[uint32]shape {
	m := map[uint32]shape{}
	add := func(sh shape, values ...uint32) {
		for _, v := range values {
			m[v] = sh
		}
	}
	add(shapeSplatI32, 15, 16, 17)
	add(shapeSplatI64, 18)
	add(shapeSplatF32, 19)
	add(shapeSplatF64, 20)
	add(shapeUnary, 77, 96, 97, 98, 103, 104, 105, 106, 116, 117, 122, 124, 125, 126, 127, 128, 129, 148, 160, 161, 192, 193, 224, 225, 227, 236, 237, 239, 248, 249, 250, 251, 257, 258)
	add(shapeShift, 107, 108, 109, 139, 140, 141, 171, 172, 173, 203, 204, 205)
	add(shapeNarrow, 101, 102, 133, 134)
	add(shapeReduceAny, 83)
	add(shapeReduceAll, 99, 131, 163, 195)
	add(shapeReduceMask, 100, 132, 164, 196)
	add(shapeCrossUnaryLow, 135, 137, 167, 169, 199, 201)
	add(shapeCrossUnaryHigh, 136, 138, 168, 170, 200, 202)
	add(shapeCrossBinaryLow, 156, 158, 188, 190, 220, 222)
	add(shapeCrossBinaryHigh, 157, 159, 189, 191, 221, 223)
	add(shapePackLow64, 94, 252, 253, 259, 260)
	add(shapeLow64Windows, 95, 254, 255)
	add(shapeSwizzle, 14, 256)
	add(shapeLoad, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 92, 93)
	add(shapeStore, 11)
	add(shapeMemLoadLane, 84, 85, 86, 87)
	add(shapeMemStoreLane, 88, 89, 90, 91)
	add(shapeConst, 12)
	add(shapeShuffle, 13)
	add(shapeExtractI32, 21, 22, 24, 25, 27)
	add(shapeExtractI64, 29)
	add(shapeExtractF32, 31)
	add(shapeExtractF64, 33)
	add(shapeReplaceI32, 23, 26, 28)
	add(shapeReplaceI64, 30)
	add(shapeReplaceF32, 32)
	add(shapeReplaceF64, 34)
	add(shapeTernary, 82, 261, 262, 263, 264, 265, 266, 267, 268, 275)
	add(shapeBinary,
		35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54,
		55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76,
		78, 79, 80, 81, 110, 111, 112, 113, 114, 115, 118, 119, 120, 121, 123, 130,
		142, 143, 144, 145, 146, 147, 149, 150, 151, 152, 153, 155, 174, 177, 181, 182, 183, 184, 185, 186,
		206, 209, 213, 214, 215, 216, 217, 218, 219, 228, 229, 230, 231, 232, 233, 234, 235,
		240, 241, 242, 243, 244, 245, 246, 247, 269, 270, 271, 272, 273, 274)
	return m
}()

func (o operation) kernelArity() (int, bool) {
	switch o.Shape {
	case shapeUnary, shapeCrossUnaryLow, shapeCrossUnaryHigh, shapePackLow64, shapeLow64Windows:
		return 1, true
	case shapeBinary, shapeNarrow, shapeCrossBinaryLow, shapeCrossBinaryHigh, shapeSwizzle:
		return 2, true
	case shapeTernary:
		return 3, true
	default:
		return 0, false
	}
}

// canonicalNames is the guest-visible semantic catalog. Numeric 0xfd
// subopcodes remain an engine-private implementation detail.
var canonicalNames = map[uint32]string{
	35: "i8x16.eq", 36: "i8x16.ne", 37: "i8x16.lt_s", 38: "i8x16.lt_u", 39: "i8x16.gt_s", 40: "i8x16.gt_u", 41: "i8x16.le_s", 42: "i8x16.le_u", 43: "i8x16.ge_s", 44: "i8x16.ge_u",
	45: "i16x8.eq", 46: "i16x8.ne", 47: "i16x8.lt_s", 48: "i16x8.lt_u", 49: "i16x8.gt_s", 50: "i16x8.gt_u", 51: "i16x8.le_s", 52: "i16x8.le_u", 53: "i16x8.ge_s", 54: "i16x8.ge_u",
	55: "i32x4.eq", 56: "i32x4.ne", 57: "i32x4.lt_s", 58: "i32x4.lt_u", 59: "i32x4.gt_s", 60: "i32x4.gt_u", 61: "i32x4.le_s", 62: "i32x4.le_u", 63: "i32x4.ge_s", 64: "i32x4.ge_u",
	65: "f32x4.eq", 66: "f32x4.ne", 67: "f32x4.lt", 68: "f32x4.gt", 69: "f32x4.le", 70: "f32x4.ge",
	71: "f64x2.eq", 72: "f64x2.ne", 73: "f64x2.lt", 74: "f64x2.gt", 75: "f64x2.le", 76: "f64x2.ge",
	77: "v128.not", 78: "v128.and", 79: "v128.andnot", 80: "v128.or", 81: "v128.xor", 82: "v128.bitselect",
	96: "i8x16.abs", 97: "i8x16.neg", 98: "i8x16.popcnt",
	103: "f32x4.ceil", 104: "f32x4.floor", 105: "f32x4.trunc", 106: "f32x4.nearest",
	116: "f64x2.ceil", 117: "f64x2.floor", 122: "f64x2.trunc", 148: "f64x2.nearest",
	124: "i16x8.extadd_pairwise_i8x16_s", 125: "i16x8.extadd_pairwise_i8x16_u",
	126: "i32x4.extadd_pairwise_i16x8_s", 127: "i32x4.extadd_pairwise_i16x8_u",
	128: "i16x8.abs", 129: "i16x8.neg", 160: "i32x4.abs", 161: "i32x4.neg", 192: "i64x2.abs", 193: "i64x2.neg",
	224: "f32x4.abs", 225: "f32x4.neg", 227: "f32x4.sqrt",
	236: "f64x2.abs", 237: "f64x2.neg", 239: "f64x2.sqrt",
	248: "i32x4.trunc_sat_f32x4_s", 249: "i32x4.trunc_sat_f32x4_u",
	250: "f32x4.convert_i32x4_s", 251: "f32x4.convert_i32x4_u",
	257: "i32x4.relaxed_trunc_f32x4_s", 258: "i32x4.relaxed_trunc_f32x4_u",
	110: "i8x16.add", 111: "i8x16.add_sat_s", 112: "i8x16.add_sat_u", 113: "i8x16.sub", 114: "i8x16.sub_sat_s", 115: "i8x16.sub_sat_u",
	118: "i8x16.min_s", 119: "i8x16.min_u", 120: "i8x16.max_s", 121: "i8x16.max_u", 123: "i8x16.avgr_u",
	130: "i16x8.q15mulr_sat_s", 142: "i16x8.add", 143: "i16x8.add_sat_s", 144: "i16x8.add_sat_u", 145: "i16x8.sub", 146: "i16x8.sub_sat_s", 147: "i16x8.sub_sat_u",
	149: "i16x8.mul", 150: "i16x8.min_s", 151: "i16x8.min_u", 152: "i16x8.max_s", 153: "i16x8.max_u", 155: "i16x8.avgr_u",
	174: "i32x4.add", 177: "i32x4.sub", 181: "i32x4.mul", 182: "i32x4.min_s", 183: "i32x4.min_u", 184: "i32x4.max_s", 185: "i32x4.max_u", 186: "i32x4.dot_i16x8_s",
	206: "i64x2.add", 209: "i64x2.sub", 213: "i64x2.mul", 214: "i64x2.eq", 215: "i64x2.ne", 216: "i64x2.lt_s", 217: "i64x2.gt_s", 218: "i64x2.le_s", 219: "i64x2.ge_s",
	228: "f32x4.add", 229: "f32x4.sub", 230: "f32x4.mul", 231: "f32x4.div", 232: "f32x4.min", 233: "f32x4.max", 234: "f32x4.pmin", 235: "f32x4.pmax",
	240: "f64x2.add", 241: "f64x2.sub", 242: "f64x2.mul", 243: "f64x2.div", 244: "f64x2.min", 245: "f64x2.max", 246: "f64x2.pmin", 247: "f64x2.pmax",
	261: "f32x4.relaxed_madd", 262: "f32x4.relaxed_nmadd", 263: "f64x2.relaxed_madd", 264: "f64x2.relaxed_nmadd",
	265: "i8x16.relaxed_laneselect", 266: "i16x8.relaxed_laneselect", 267: "i32x4.relaxed_laneselect", 268: "i64x2.relaxed_laneselect",
	269: "f32x4.relaxed_min", 270: "f32x4.relaxed_max", 271: "f64x2.relaxed_min", 272: "f64x2.relaxed_max",
	273: "i16x8.relaxed_q15mulr_s", 274: "i16x8.relaxed_dot_i8x16_i7x16_s", 275: "i32x4.relaxed_dot_i8x16_i7x16_add_s",
}

func instructionName(bits uint16, sub uint32) (string, bool) {
	base, ok := canonicalNames[sub]
	if !ok {
		return "", false
	}
	scale := int(bits / 128)
	replacer := strings.NewReplacer(
		"v128", "v"+itoa(int(bits)),
		"i8x16", "i8x"+itoa(16*scale),
		"i7x16", "i7x"+itoa(16*scale),
		"i16x8", "i16x"+itoa(8*scale),
		"i32x4", "i32x"+itoa(4*scale),
		"i64x2", "i64x"+itoa(2*scale),
		"f32x4", "f32x"+itoa(4*scale),
		"f64x2", "f64x"+itoa(2*scale),
	)
	return replacer.Replace(base), true
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}

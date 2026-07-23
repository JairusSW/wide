(module
  (import "as-simd" "v256.load"
    (func $v256.load (param i32) (result externref)))
  (import "as-simd" "i32x8.add"
    (func $i32x8.add (param externref externref) (result externref)))
  (import "as-simd" "v256.store"
    (func $v256.store (param externref i32)))

  (memory (export "memory") 1)

  ;; Eight little-endian i32 values: [1, 2, 3, 4, 5, 6, 7, 8].
  (data (i32.const 32)
    "\01\00\00\00\02\00\00\00\03\00\00\00\04\00\00\00"
    "\05\00\00\00\06\00\00\00\07\00\00\00\08\00\00\00")

  ;; Eight little-endian i32 values: [10, 20, 30, 40, 50, 60, 70, 80].
  (data (i32.const 64)
    "\0a\00\00\00\14\00\00\00\1e\00\00\00\28\00\00\00"
    "\32\00\00\00\3c\00\00\00\46\00\00\00\50\00\00\00")

  (func (export "run")
    i32.const 32
    call $v256.load
    i32.const 64
    call $v256.load
    call $i32x8.add
    i32.const 0
    call $v256.store))

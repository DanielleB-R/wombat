(module
  (type $t0 (func))
  (type $t1 (func (param i32 i64) (result f64)))
  (table $tab 2 funcref)
  (memory $mem 1 2)
  (global $counter i32 (i32.const 42))
  (global $f f64 (f64.const 1.5))
  (elem (i32.const 0) $helper $start)
  (data (i32.const 8) "hello\00\ff wombat")
  (func $helper (type $t0) nop)
  (func $start (type $t0) nop)
  (func (export "call_it") (param i32) (result i32)
    (local i32 i32 f64 f64)
    local.get 0
    call_indirect (type $t0)
    i32.const 4
    i32.load offset=4 align=2
    i32.const 7
    i32.store8
    memory.size
    drop
    i32.const 1
    memory.grow
    drop
    i64.const -1
    f32.const 0x1p-3
    f64.const -0.0
    drop drop drop
    local.get 0
    local.get 0
    br_table 0 0)
  (export "helper" (func $helper))
  (export "counter" (global $counter))
  (export "memory" (memory $mem))
  (export "table" (table $tab))
  (export "euro_€" (func $helper))
  (start $start))

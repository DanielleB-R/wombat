(module
  (type $t1 (func (param i32 i64) (result f64)))
  (import "env" "log" (func $log (param i32)))
  (import "env" "conv" (func $conv (type $t1)))
  (import "env" "mem" (memory 1 4))
  (import "env" "tab" (table 2 funcref))
  (import "env" "g" (global i32))
  (func (export "go") (param i32)
    local.get 0
    call $log))

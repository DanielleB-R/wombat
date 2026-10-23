(module
  (func (export "fib") (param $n i32) (result i64)
    (local $a i64) (local $b i64) (local $t i64)
    i64.const 1
    local.set $b
    block $done
      loop $again
        local.get $n
        i32.eqz
        br_if $done
        local.get $a
        local.get $b
        i64.add
        local.set $t
        local.get $b
        local.set $a
        local.get $t
        local.set $b
        local.get $n
        i32.const 1
        i32.sub
        local.set $n
        br $again
      end
    end
    local.get $a))

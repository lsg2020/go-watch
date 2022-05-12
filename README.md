# go-watch
* 使用[delve](https://github.com/go-delve/delve)查找调试符号,执行修改未导出的私有函数及全局变量
* 使用反射运行时打印修改程序内部状态,方便调试
* 使用方来保证线程安全

## 注意
* 内联优化过的函数会找不到,可以使用`go build -gcflags=all=-l`关闭内联优化
* 目前测试过的版本go 1.14-1.17


## 快速使用
* 引入包 `import "github.com/lsg2020/go-watch"`
* 创建lua vm `state, err := go_watch.NewLuaState(root, print)`
    * `root`: `func(name string) interface{}` 根据name返回root数据
    * `print`: `func(session int, str string)` lua print函数的输出回调
* 执行打印修复的lua脚本 `err := go_watch.Execute(state, script)`
    * `state`: lua vm
    * `script`: 对应的lua脚本

## 示例

* [打印状态](https://github.com/lsg2020/go-watch/blob/master/examples/modify.go)

``` lua
local go_watch = require('go_watch') -- 引入包
local root = go_watch.root_get('')   -- 获取root数据

-- get unexport field
print("get unexport field TestData.name:", go_watch.get_string(go_watch.field_get_by_name(root, "name")))
```

* [修改字段](https://github.com/lsg2020/go-watch/blob/master/examples/modify.go)

```lua
local go_watch = require('go_watch') -- 引入包
local root = go_watch.root_get('')   -- 获取root数据

-- modify unexport field
local map1 = go_watch.field_get_by_name(root, "map1")
local role1 = go_watch.map_get(map1, go_watch.new_int32(1))
go_watch.field_set_by_name(role1, "name", go_watch.new_string("MODIFY BY LUA role1"))
```

* [调用函数](https://github.com/lsg2020/go-watch/blob/master/examples/function.go)

```lua
local go_watch = require('go_watch') -- 引入包
local role1 = go_watch.root_get('')  -- 获取root数据

-- call unexport function
local r1, r2, r3 = go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", {go_watch.new_int(1), go_watch.new_int(2)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
print("call_func_with_name testAdd:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

-- call unexport method
go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", {role1, go_watch.new_string("Name by lua")}, {})
```


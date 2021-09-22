# go-watch
* 使用[monkey patching](https://github.com/bouk/monkey)配合lua脚本运行时替换修复函数
* 使用[go-forceexport](https://github.com/AlaxLee/go-forceexport)执行未导出的私有函数
* 使用反射运行时打印修改程序内部状态,方便调试

## 注意
* 内联优化过的函数会找不到,可以使用`go run -gcflags=all=-l`关闭内联优化
* 修复及执行函数并不十分安全,生产环境谨慎使用
* 目前测试过的版本go 1.14/1.15/1.16


## 快速使用
* 引入包 `import "github.com/lsg2020/go-watch"`
* 创建lua vm `state := go_watch.NewLuaState(root, print)`
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
local r1, r2, r3 = go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", {go_watch.new_int(1), go_watch.new_int(2)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
print("call_with_name testAdd:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

-- call unexport method
go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", {role1, go_watch.new_string("Name by lua")}, {})
```

* [修复函数](https://github.com/lsg2020/go-watch/blob/master/examples/hotfix.go)

``` lua
local go_watch = require('go_watch')
local role1 = go_watch.root_get('')

-- call unexport function
local r1, r2, r3 = go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", {go_watch.new_int(1), go_watch.new_int(2)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
print("call_with_name testAdd:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

-- hotfix unexport function
go_watch.hotfix_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", [[
    local go_watch = require('go_watch') 
    local a, b = ...
    print("hotfix replace testAdd:", go_watch.get_number(a), go_watch.get_number(b))
    return a, b, go_watch.new_int(go_watch.get_number(a) + go_watch.get_number(b) + 1000)
]], {go_watch.new_int(0), go_watch.new_int(0)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})

r1, r2, r3 = go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", {go_watch.new_int(1), go_watch.new_int(2)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
print("hotfix testAdd:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))


-- call unexport method
go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", {role1, go_watch.new_string("Name by lua")}, {})

-- hotfix unexport method
go_watch.hotfix_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", [[
    local go_watch = require('go_watch') 
    local role, name = ...
    name = go_watch.get_string(name)
    print("hotfix replace setName  oldName:", go_watch.get_string(go_watch.field_get_by_name(role, "name")), " newName:", name)
    go_watch.field_set_by_name(role, "name", go_watch.new_string("hotfix name ------" .. name))
]], {go_watch.new_with_name("module_data.RoleInfo", true), go_watch.new_string("")}, {})

-- call unexport method
go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", {role1, go_watch.new_string("Name by lua")}, {})
```

## Thanks

[https://github.com/bouk/monkey](https://github.com/bouk/monkey)

[https://github.com/AlaxLee/go-forceexport](https://github.com/AlaxLee/go-forceexport)

[https://github.com/v2pro/plz](https://github.com/v2pro/plz)

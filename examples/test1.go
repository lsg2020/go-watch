package main

import (
	"fmt"

	go_watch "github.com/lsg2020/go-watch"
	"github.com/lsg2020/go-watch/examples/module_data"
)

var data *module_data.TestData = module_data.NewData("TEST DATA NAME")

type test struct {
}

func (p *test) Get(name string) interface{} {
	return data
}

func (p *test) Print(session int, str string) {
	fmt.Println("debug lua print:  ", session, str)
}

func main() {
	state := go_watch.NewLuaState(&test{})
	defer state.Close()

	data.AddMapRole(1, module_data.NewRole(1, "role1", 1))
	data.AddMapRole(2, module_data.NewRole(2, "role2", 2))

	fmt.Printf("role1: %#v\n", data.GetMapRole(1))

	if err := go_watch.Execute(state, `
	local go_watch = require('go_watch')
	local root = go_watch.root_get('')

	print("\n\n")
	print("data name:", go_watch.get_string(go_watch.field_get_by_name(root, "name")))
	
	local map1 = go_watch.field_get_by_name(root, "map1")

	-- test add map
	local role10000 = go_watch.map_new_val(map1)
	go_watch.field_set_by_name(role10000, "name", go_watch.new_string("lua role 1"))
	go_watch.field_set_by_name(role10000, "level", go_watch.new_int32(10000))
	go_watch.map_set(map1, go_watch.new_int32(10000), role10000)

	-- test modify map
	local role1 = go_watch.map_get(map1, go_watch.new_int32(1))
	go_watch.field_set_by_name(role1, "name", go_watch.new_string("MODIFY BY LUA role1"))

	-- test map foreach
	go_watch.map_foreach(map1, function(k, v)
		print("lua map foreach", go_watch.get_number(k), go_watch.get_string(go_watch.field_get_by_name(v, "name")))
	    return 1
	end)
	
	-- test append slice
	local slice1 = go_watch.field_get_by_name(root, "slice1")
	local role100 = go_watch.array_new_elem(slice1)
	go_watch.field_set_by_name(role100, "name", go_watch.new_string("role100"))
	go_watch.field_set_by_name(role100, "level", go_watch.new_int32(100))
	go_watch.field_set_by_name(root, "slice1", go_watch.slice_append(slice1, role100))

	print("\n\n")

	local add1 = go_watch.method_get_by_name(role1, "Add")
	local r1, r2, r3 = go_watch.call(add1, go_watch.new_int(100), go_watch.new_int(200))
	print("call:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

	r1, r2, r3 = go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", {go_watch.new_int(101), go_watch.new_int(201)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
	print("call_with_name:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

	r1, r2, r3 = go_watch.call_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).add", {role1, go_watch.new_int(102), go_watch.new_int(202)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
	print("call_with_name:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

	`, 1); err != nil {
		fmt.Println("go watch error:", err)
		return
	}

	fmt.Printf("lua role: %#v\n", data.GetMapRole(10000))
	fmt.Printf("new role1: %#v\n", data.GetMapRole(1))
}

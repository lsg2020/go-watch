package main

import (
	"fmt"

	go_watch "github.com/lsg2020/go-watch"
	"github.com/lsg2020/go-watch/examples/module_data"
)

var data = module_data.NewRole(1, "", 1)

func main() {
	state, err := go_watch.NewLuaState(func(name string) interface{} {
		return data
	}, func(session int, str string) {
		fmt.Println("lua print:", session, str)
	})
	defer state.Close()

	if err != nil {
		panic(err)
	}

	fmt.Printf("%#v\n", data)

	if err := go_watch.Execute(state, `
	local go_watch = require('go_watch')
	local role1 = go_watch.root_get('')

	local module_types = go_watch.search_type_name("module_data")
	for i, v in pairs(module_types) do
		print("type found:", i, v)
	end

	-- call unexport function
	local r1, r2, r3 = go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", {go_watch.new_int(1), go_watch.new_int(2)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
	print("call_func_with_name testAdd:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

	-- hotfix unexport function
	go_watch.hotfix_func_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", [[
		local go_watch = require('go_watch') 
		local a, b = ...
		print("hotfix replace testAdd:", go_watch.get_number(a), go_watch.get_number(b))
		return a, b, go_watch.new_int(go_watch.get_number(a) + go_watch.get_number(b) + 1000)
	]], {go_watch.new_int(0), go_watch.new_int(0)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})

	r1, r2, r3 = go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", {go_watch.new_int(1), go_watch.new_int(2)}, {go_watch.new_int(0), go_watch.new_int(0), go_watch.new_int(0)})
	print("hotfix testAdd:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))


	-- call unexport method
	go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", {role1, go_watch.new_string("Name by lua")}, {})

	-- hotfix unexport method
	go_watch.hotfix_func_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", [[
		local go_watch = require('go_watch') 
		local role, name = ...
		name = go_watch.get_string(name)
		print("hotfix replace setName  oldName:", go_watch.get_string(go_watch.field_get_by_name(role, "name")), " newName:", name)
		go_watch.field_set_by_name(role, "name", go_watch.new_string("hotfix name ------" .. name))
	]], {go_watch.new_with_name("github.com/lsg2020/go-watch/examples/module_data.RoleInfo", true), go_watch.new_string("")}, {})

	-- call unexport method
	go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", {role1, go_watch.new_string("Name by lua")}, {})


	`, 1); err != nil {
		fmt.Println("go watch error:", err)
		return
	}

	fmt.Printf("%#v\n", data)

}

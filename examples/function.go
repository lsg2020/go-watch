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

	-- call unexport function
	local r1, r2, r3 = go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.testAdd", false, {go_watch.new_int(1), go_watch.new_int(2)})
	print("call_func_with_name testAdd:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

	local module_funcs = go_watch.search_func_name("module_data")
	for i, v in pairs(module_funcs) do
		print("func found:", i, v)
	end

	-- call unexport method
	go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).setName", false, {role1, go_watch.new_string("Name by lua")})

	-- call public method
	local r1, r2, r3 = go_watch.call_func_with_name("github.com/lsg2020/go-watch/examples/module_data.(*RoleInfo).Add", false, {role1, go_watch.new_int(100), go_watch.new_int(200)})
	print("call RoleInfo.Add:", go_watch.get_number(r1), go_watch.get_number(r2), go_watch.get_number(r3))

	`, 1); err != nil {
		fmt.Println("go watch error:", err)
		return
	}

	fmt.Printf("%#v\n", data)

}

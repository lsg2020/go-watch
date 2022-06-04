package main

import (
	"fmt"

	go_watch "github.com/lsg2020/go-watch"
	"github.com/lsg2020/go-watch/examples/module_data"
)

func main() {
	state, err := go_watch.NewLuaState(func(name string) interface{} {
		return nil
	}, func(session int, str string) {
		fmt.Println("lua print:", session, str)
	})
	defer state.Close()

	if err != nil {
		panic(err)
	}

	fmt.Println("=====", *module_data.GetGlobal())

	if err := go_watch.Execute(state, `
	local go_watch = require('go_watch')
	local root = go_watch.root_get('')

	local module_globals = go_watch.search_global_name("module_data")
	for i, v in pairs(module_globals) do
		print("global found:", i, v)
	end

	local global = go_watch.get_global_with_name("github.com/lsg2020/go-watch/examples/module_data.testGlobalRoleInfo")
	print("1234", go_watch.to_string(global), go_watch.get_string(go_watch.rval_to_interface(go_watch.interface_to_rval(go_watch.new_string("--==")))))
	global = go_watch.val_to_ptr(global)
	go_watch.field_set_by_name(global, "name", go_watch.new_string("test set global name"))


	`, 1); err != nil {
		fmt.Println("go watch error:", err)
		return
	}

	fmt.Println("=====", *module_data.GetGlobal())
}

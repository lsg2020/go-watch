package main

import (
	"fmt"

	go_watch "github.com/lsg2020/go-watch"
	"github.com/lsg2020/go-watch/examples/module_data"
)

var data *module_data.TestData = module_data.NewData("TEST DATA NAME")

func main() {
	state := go_watch.NewLuaState(func(name string) interface{} {
		return data
	}, func(session int, str string) {
		fmt.Println("lua print:", session, str)
	})
	defer state.Close()

	fmt.Printf("%#v\n", data)

	if err := go_watch.Execute(state, `
	local go_watch = require('go_watch')
	local root = go_watch.root_get('')

	-- get unexport field
	print("get unexport field TestData.name:", go_watch.get_string(go_watch.field_get_by_name(root, "name")))
	
	-- map add
	local map1 = go_watch.field_get_by_name(root, "map1")
	for i = 1, 5 do
		local role = go_watch.map_new_val(map1)
		go_watch.field_set_by_name(role, "name", go_watch.new_string("lua role "..i))
		go_watch.field_set_by_name(role, "level", go_watch.new_int32(i))
		go_watch.map_set(map1, go_watch.new_int32(i), role)
	end

	-- modify unexport field
	do
		local role1 = go_watch.map_get(map1, go_watch.new_int32(1))
		go_watch.field_set_by_name(role1, "name", go_watch.new_string("MODIFY BY LUA role1"))
	end

	-- map foreach
	go_watch.map_foreach(map1, function(k, v)
		print("map foreach", go_watch.get_number(k), go_watch.get_string(go_watch.field_get_by_name(v, "name")))
	    return 1
	end)
	
	-- test append slice
	local slice1 = go_watch.field_get_by_name(root, "slice1")
	for i = 1, 5 do
		local role = go_watch.array_new_elem(slice1)
		go_watch.field_set_by_name(role, "name", go_watch.new_string("role:"..i))
		go_watch.field_set_by_name(role, "level", go_watch.new_int32(i))
		go_watch.field_set_by_name(root, "slice1", go_watch.slice_append(slice1, go_watch.clone(role, false)))
	end

	`, 1); err != nil {
		fmt.Println("go watch error:", err)
		return
	}

	fmt.Printf("%#v\n", data)

}

package go_watch

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"bou.ke/monkey"
	lua "github.com/yuin/gopher-lua"
	"github.com/zeebo/goof"
)

const module_name = "go_watch"
const debug_ctx = "go_watch_debug_ctx"

var exports map[string]lua.LGFunction
var goof_troop goof.Troop

func init() {
	exports = map[string]lua.LGFunction{
		"root_get": root_get,
		"print":    print,

		"search_type_name":     search_type_name,
		"search_func_name":     search_func_name,
		"search_global_name":   search_global_name,
		"get_type_with_name":   get_type_with_name,
		"get_obj_type":         get_obj_type,
		"get_global_with_name": get_global_with_name,

		"clone":                 clone,
		"ptr_to_val":            ptr_to_val,
		"val_to_ptr":            val_to_ptr,
		"convert_type_to":       convert_type_to,
		"call":                  call,
		"call_func_with_name":   call_func_with_name,
		"hotfix_func_with_name": hotfix_func_with_name,

		"field_get_by_name":  field_get_by_name,
		"field_set_by_name":  field_set_by_name,
		"method_get_by_name": method_get_by_name,

		"map_get":     map_get,
		"map_set":     map_set,
		"map_del":     map_del,
		"map_foreach": map_foreach,
		"map_new_key": map_new_key,
		"map_new_val": map_new_val,

		"array_new_elem": array_new_elem,
		"array_foreach":  array_foreach,
		"array_get":      array_get,
		"array_set":      array_set,
		"array_slice":    array_slice,
		"slice_append":   slice_append,

		"get_string":  get_string,
		"set_string":  set_string,
		"get_number":  get_number,
		"set_number":  set_number,
		"get_boolean": get_boolean,
		"set_boolean": set_boolean,
		"set_any":     set_any,

		"new_boolean":   new_boolean,
		"new_int":       new_int,
		"new_int8":      new_int8,
		"new_int16":     new_int16,
		"new_int32":     new_int32,
		"new_int64":     new_int64,
		"new_uint8":     new_uint8,
		"new_uint16":    new_uint16,
		"new_uint32":    new_uint32,
		"new_uint64":    new_uint64,
		"new_string":    new_string,
		"new_with_name": new_with_name,
		"new_interface": new_interface,
	}
}

type Func struct {
	codePtr uintptr
}

type RootFunc func(name string) interface{}
type PrintFunc func(session int, str string)
type Context struct {
	root  RootFunc
	print PrintFunc
}

func NewLuaState(root RootFunc, print PrintFunc) (*lua.LState, error) {
	_, err := goof_troop.Global("")
	if err != nil {
		return nil, err
	}

	state := lua.NewState()
	ud := new_userdata(state, &Context{root: root, print: print})
	state.SetGlobal(debug_ctx, ud)

	state.PreloadModule(module_name, func(state *lua.LState) int {
		mod := state.SetFuncs(state.NewTable(), exports)
		state.Push(mod)
		return 1
	})

	return state, nil
}

func Execute(state *lua.LState, script string, session int) error {
	code_template := `
		local go_watch = require("go_watch")
		local session = %d
		local function debug_print(...)
			local args = {...}
			local out = {}
			for k, v in ipairs(args) do
				out[k] = tostring(v)
			end
			out = table.concat(out, '\t')
			go_watch.print(session, out)
		end
		local env = setmetatable({print=debug_print}, {__index=_G})
		local f = loadstring(%q)
		setfenv(f, env)
		local r, err = xpcall(f, debug.traceback)
		if not r then
			go_watch.print(session, err)
			return
		end
	`

	fn, err := state.LoadString(`local template, session, script = ...; return string.format(template, session, script); `)
	if err != nil {
		return err
	}

	state.Push(fn)
	state.Push(lua.LString(code_template))
	state.Push(lua.LNumber(session))
	state.Push(lua.LString(script))

	err = state.PCall(3, 1, nil)
	if err != nil {
		return err
	}

	code := state.CheckString(-1)
	state.Pop(1)

	err = state.DoString(code)
	return err

}

func new_userdata(state *lua.LState, data interface{}) *lua.LUserData {
	ud := state.NewUserData()
	ud.Value = data
	return ud
}

func get_context(state *lua.LState) (ctx *Context) {
	ud, ok := state.GetGlobal(debug_ctx).(*lua.LUserData)
	if !ok {
		state.RaiseError("debug_ctx error")
	}

	ctx, ok = ud.Value.(*Context)
	if !ok {
		state.RaiseError("debug_ctx error.")
	}
	return
}

func root_get(state *lua.LState) int {
	ctx := get_context(state)
	name := state.CheckString(1)

	ud := new_userdata(state, ctx.root(name))
	state.Push(ud)
	return 1
}

func print(state *lua.LState) int {
	ctx := get_context(state)

	session := state.CheckNumber(1)
	str := state.CheckString(2)

	ctx.print(int(session), str)
	return 0
}

func call(state *lua.LState) int {
	ud := state.CheckUserData(1)
	paramList := make([]reflect.Value, 0, state.GetTop()-1)
	for i := 1; i < state.GetTop(); i++ {
		p := state.CheckUserData(i + 1)
		if r, ok := p.Value.(reflect.Value); ok {
			paramList = append(paramList, r)
		} else {
			paramList = append(paramList, reflect.ValueOf(p.Value))
		}
	}

	var rfn reflect.Value
	if r, ok := ud.Value.(reflect.Value); ok {
		rfn = r
	} else {
		rfn = reflect.ValueOf(ud.Value)
	}

	var ret []reflect.Value
	if rfn.Kind() == reflect.Ptr && rfn.Elem().Kind() == reflect.Func {
		ret = rfn.Elem().Call(paramList)
	} else if rfn.Kind() == reflect.Func {
		ret = rfn.Call(paramList)
	} else {
		state.RaiseError("param1 need function")
	}

	for _, r := range ret {
		ud := new_userdata(state, r)
		state.Push(ud)
	}

	return len(ret)
}

var (
	ErrNotFound = errors.New("not found")
)

var types map[string]reflect.Type
var types_mutex sync.Mutex

//go:linkname dwarfName github.com/zeebo/goof.dwarfName
func dwarfName(rtyp reflect.Type) (out string)

func load_types() {
	types_mutex.Lock()
	defer types_mutex.Unlock()

	if types != nil {
		return
	}

	all_type, _ := goof_troop.Types()
	types = make(map[string]reflect.Type, len(all_type))
	for _, t := range all_type {
		types[dwarfName(t)] = t
	}
}

func find_func_with_name(name string) (uintptr, error) {
	rtroop := reflect.ValueOf(goof_troop)
	functions := rtroop.FieldByName("functions")
	if !functions.IsValid() {
		return 0, ErrNotFound
	}
	fun := functions.MapIndex(reflect.ValueOf(name))
	if !fun.IsValid() {
		return 0, ErrNotFound
	}

	pc := fun.FieldByName("pc")
	return uintptr(pc.Uint()), nil
}

func call_func_with_name(state *lua.LState) int {
	name := state.CheckString(1)
	in := state.CheckTable(2)
	out := state.CheckTable(3)

	ptr, err := find_func_with_name(name)
	if err != nil {
		state.RaiseError(fmt.Sprintf("func:%s not found", name))
	}

	in_types := make([]reflect.Type, in.Len())
	in_values := make([]reflect.Value, in.Len())
	for i := 1; i <= in.Len(); i++ {
		v := in.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				in_types[i-1] = r.Type()
				in_values[i-1] = r
			} else {
				in_types[i-1] = reflect.TypeOf(ud.Value)
				in_values[i-1] = reflect.ValueOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("in params:%d not user data", i))
		}
	}

	out_types := make([]reflect.Type, out.Len())
	for i := 1; i <= out.Len(); i++ {
		v := out.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				out_types[i-1] = r.Type()
			} else if t, ok := ud.Value.(reflect.Type); ok {
				out_types[i-1] = t
			} else {
				out_types[i-1] = reflect.TypeOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("out params:%d not user data", i))
		}
	}

	new_func := reflect.MakeFunc(reflect.FuncOf(in_types, out_types, false), nil)
	func_ptr_val := reflect.ValueOf(new_func).FieldByName("ptr").Pointer()
	func_ptr := (*Func)(unsafe.Pointer(func_ptr_val))
	func_ptr.codePtr = ptr

	ret := new_func.Call(in_values)
	for _, r := range ret {
		ud := new_userdata(state, r)
		state.Push(ud)
	}
	return len(ret)
}

type HotfixContext struct {
	state *lua.LState
	fn    *lua.LFunction
	in    []reflect.Type
	out   []reflect.Type
	lock  sync.Mutex
}

func (ctx *HotfixContext) Do(params []reflect.Value) []reflect.Value {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	var err error
	ret := make([]reflect.Value, len(ctx.out))
	if len(params) != len(ctx.in) {
		goto err_ret
	}

	ctx.state.Push(ctx.fn)
	for _, param := range params {
		ctx.state.Push(new_userdata(ctx.state, param))
	}

	err = ctx.state.PCall(len(params), len(ctx.out), nil)
	if err != nil {
		fmt.Println("hotfix function error", err)
		goto err_ret
	}

	for i := 1; i <= len(ctx.out); i++ {
		ud := ctx.state.CheckUserData(-1)
		ctx.state.Pop(1)

		if r, ok := ud.Value.(reflect.Value); ok {
			ret[len(ctx.out)-i] = r
		} else {
			ret[len(ctx.out)-i] = reflect.ValueOf(ud.Value)
		}
	}
	return ret

err_ret:

	for i, o := range ctx.out {
		ret[i] = reflect.New(o).Elem()
	}
	return ret
}

func search_func_name(state *lua.LState) int {
	include := state.CheckString(1)
	functions, err := goof_troop.Functions()
	if err != nil {
		state.RaiseError(fmt.Sprintf("system error %s", err.Error()))
	}

	ret := &lua.LTable{}
	for _, name := range functions {
		if include == "" || strings.Index(name, include) >= 0 {
			ret.Append(lua.LString(name))
		}
	}

	state.Push(ret)
	return 1
}

func search_global_name(state *lua.LState) int {
	include := state.CheckString(1)
	globals, err := goof_troop.Globals()
	if err != nil {
		state.RaiseError(fmt.Sprintf("system error %s", err.Error()))
	}

	ret := &lua.LTable{}
	for _, name := range globals {
		if include == "" || strings.Index(name, include) >= 0 {
			ret.Append(lua.LString(name))
		}
	}

	state.Push(ret)
	return 1
}

func hotfix_func_with_name(state *lua.LState) int {
	name := state.CheckString(1)
	script := state.CheckString(2)
	in := state.CheckTable(3)
	out := state.CheckTable(4)

	ptr, err := find_func_with_name(name)
	if err != nil {
		state.RaiseError(fmt.Sprintf("func:%s not found", name))
	}

	ctx := get_context(state)
	new_state, _ := NewLuaState(ctx.root, ctx.print)

	fn, err := new_state.LoadString(script)
	if err != nil {
		state.RaiseError(fmt.Sprintf("script error:%v", err))
	}

	in_types := make([]reflect.Type, in.Len())
	for i := 1; i <= in.Len(); i++ {
		v := in.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				in_types[i-1] = r.Type()
			} else if t, ok := ud.Value.(reflect.Type); ok {
				in_types[i-1] = t
			} else {
				in_types[i-1] = reflect.TypeOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("in params:%d not user data", i))
		}
	}

	out_types := make([]reflect.Type, out.Len())
	for i := 1; i <= out.Len(); i++ {
		v := out.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				out_types[i-1] = r.Type()
			} else if t, ok := ud.Value.(reflect.Type); ok {
				out_types[i-1] = t
			} else {
				out_types[i-1] = reflect.TypeOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("out params:%d not user data", i))
		}
	}

	old_func := reflect.MakeFunc(reflect.FuncOf(in_types, out_types, false), nil)
	func_ptr_val := reflect.ValueOf(old_func).FieldByName("ptr").Pointer()
	func_ptr := (*Func)(unsafe.Pointer(func_ptr_val))
	func_ptr.codePtr = ptr

	hotfix := &HotfixContext{state: new_state, fn: fn, in: in_types, out: out_types}
	new_func := reflect.MakeFunc(reflect.FuncOf(in_types, out_types, false), hotfix.Do)
	monkey.Patch(old_func.Interface(), new_func.Interface())
	return 0
}

func method_get_by_name(state *lua.LState) int {
	ud := state.CheckUserData(1)
	name := state.CheckString(2)

	var rf reflect.Value
	var rud reflect.Value
	if r, ok := ud.Value.(reflect.Value); ok {
		rud = r
	} else {
		rud = reflect.ValueOf(ud.Value)
	}
	if rud.Kind() == reflect.Ptr && (rud.Elem().Kind() == reflect.Struct || rud.Elem().Kind() == reflect.Interface) {
		rf = rud.MethodByName(name)
	} else if rud.Kind() == reflect.Struct || rud.Kind() == reflect.Interface {
		rf = rud.MethodByName(name)
	} else {
		state.RaiseError("param1 need struct/interface")
	}

	ret := new_userdata(state, rf)
	state.Push(ret)
	return 1
}

func field_get_by_name(state *lua.LState) int {
	ud := state.CheckUserData(1)
	name := state.CheckString(2)

	var rf reflect.Value
	var rud reflect.Value
	if r, ok := ud.Value.(reflect.Value); ok {
		rud = r
	} else {
		rud = reflect.ValueOf(ud.Value)
	}
	if rud.Kind() == reflect.Ptr && rud.Elem().Kind() == reflect.Struct {
		rs := rud.Elem()
		rf = rs.FieldByName(name)
		rf = reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()
	} else if rud.Kind() == reflect.Struct {
		/*
			rs := rud
			rs2 := reflect.New(rs.Type()).Elem()
			rs2.Set(rs)
			rf = rs2.FieldByName(name)
			rf = reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()
		*/
		rf = rud.FieldByName(name)
	} else {
		state.RaiseError("param1 need struct")
	}

	ret := new_userdata(state, rf)
	state.Push(ret)
	return 1
}

func field_set_by_name(state *lua.LState) int {
	ud := state.CheckUserData(1)
	name := state.CheckString(2)
	new_val := state.CheckUserData(3)

	var rf reflect.Value
	var rud reflect.Value
	if r, ok := ud.Value.(reflect.Value); ok {
		rud = r
	} else {
		rud = reflect.ValueOf(ud.Value)
	}
	if rud.Kind() == reflect.Ptr && rud.Elem().Kind() == reflect.Struct {
		rs := rud.Elem()
		rf = rs.FieldByName(name)
		rf = reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()
	} else if rud.Kind() == reflect.Struct {
		rf = rud.FieldByName(name)
	} else {
		state.RaiseError("param1 need struct")
	}

	if rf.Kind() == reflect.Ptr {
		rf = rf.Elem()
	}
	if rn, ok := new_val.Value.(reflect.Value); ok {
		rf.Set(rn)
	} else {
		rf.Set(reflect.ValueOf(new_val.Value))
	}
	return 0
}

func ptr_to_val(state *lua.LState) int {
	ud := state.CheckUserData(1)
	var rf reflect.Value
	if r, ok := ud.Value.(reflect.Value); ok {
		rf = r
	} else {
		rf = reflect.ValueOf(ud.Value)
	}
	if rf.Kind() != reflect.Ptr {
		state.RaiseError("param1 need pointer")
	}

	state.Push(new_userdata(state, rf.Elem()))
	return 1
}

func val_to_ptr(state *lua.LState) int {
	ud := state.CheckUserData(1)
	var rf reflect.Value
	if r, ok := ud.Value.(reflect.Value); ok {
		rf = r
	} else {
		rf = reflect.ValueOf(ud.Value)
	}

	if !rf.CanAddr() {
		state.RaiseError("param1 convert pointer error")
	}

	state.Push(new_userdata(state, rf.Addr()))
	return 1
}

func clone(state *lua.LState) int {
	ud := state.CheckUserData(1)
	//ret_ptr := state.CheckBool(2)

	var new_rs reflect.Value
	rud, ok := ud.Value.(reflect.Value)
	if !ok {
		rud = reflect.ValueOf(ud.Value)
	}

	new_rs = reflect.New(rud.Type())
	new_rs.Elem().Set(rud)
	state.Push(new_userdata(state, new_rs.Elem()))
	return 1

	/*
		if rud.Kind() == reflect.Ptr {
			rs := rud.Elem()
			new_rs = reflect.New(rs.Type())
			new_rs.Elem().Set(rs)
		} else {
			rs := rud
			new_rs = reflect.New(rs.Type())
			new_rs.Elem().Set(rs)
		}

		var ret *lua.LUserData
		if ret_ptr {
			ret = new_userdata(state, new_rs)
		} else {
			ret = new_userdata(state, new_rs.Elem())
		}
		state.Push(ret)
		return 1
	*/
}

func convert_type_to(state *lua.LState) int {
	src_ud := state.CheckUserData(1)
	to_ud := state.CheckUserData(2)
	src, ok := src_ud.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}
	if src.Kind() != reflect.Ptr && src.Kind() != reflect.Interface {
		state.RaiseError("param1 need ptr/interface")
	}
	var t reflect.Type
	switch to := to_ud.Value.(type) {
	case reflect.Value:
		t = to.Type()
	case reflect.Type:
		t = to
	default:
		state.RaiseError("param2 need reflect.Value/reflect.Type")
	}

	ret := src.Elem().Convert(t)
	state.Push(new_userdata(state, ret))
	return 1
}

func map_get(state *lua.LState) int {
	m := state.CheckUserData(1)
	k := state.CheckUserData(2)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Map {
		state.RaiseError(fmt.Sprintf("field is %s need map type", rf.Type().Name()))
	}

	var krf reflect.Value
	krf, ok = k.Value.(reflect.Value)
	if !ok {
		krf = reflect.ValueOf(k.Value)
	}

	ret := rf.MapIndex(krf)
	if !ret.IsValid() {
		return 0
	}
	// ret.CanInterface
	state.Push(new_userdata(state, ret))
	return 1
}

func map_set(state *lua.LState) int {
	m := state.CheckUserData(1)
	k := state.CheckUserData(2)
	v := state.CheckUserData(3)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Map {
		state.RaiseError(fmt.Sprintf("field is %s need map type", rf.Type().Name()))
	}

	var krf reflect.Value
	krf, ok = k.Value.(reflect.Value)
	if !ok {
		krf = reflect.ValueOf(k.Value)
	}

	if vrf, ok := v.Value.(reflect.Value); ok {
		rf.SetMapIndex(krf, vrf)
	} else {
		rf.SetMapIndex(krf, reflect.ValueOf(v.Value))
	}
	return 0
}

func map_del(state *lua.LState) int {
	m := state.CheckUserData(1)
	k := state.CheckUserData(2)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Map {
		state.RaiseError(fmt.Sprintf("field is %s need map type", rf.Type().Name()))
	}

	var krf reflect.Value
	krf, ok = k.Value.(reflect.Value)
	if !ok {
		krf = reflect.ValueOf(k.Value)
	}
	rf.SetMapIndex(krf, reflect.Value{})
	return 0
}

func map_foreach(state *lua.LState) int {
	m := state.CheckUserData(1)
	cb := state.CheckFunction(2)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Map {
		state.RaiseError(fmt.Sprintf("field is %s need map type", rf.Type().Name()))
	}

	iter := rf.MapRange()
	for iter.Next() {
		k := iter.Key()
		v := iter.Value()

		state.Push(cb)
		state.Push(new_userdata(state, k))
		state.Push(new_userdata(state, v))
		state.Call(2, 1)

		ret := state.CheckNumber(-1)
		state.Pop(1)

		if ret != 1 {
			break
		}
	}

	return 0
}

func map_new_key(state *lua.LState) int {
	m := state.CheckUserData(1)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Map {
		state.RaiseError(fmt.Sprintf("field is %s need map type", rf.Type().Name()))
	}

	var v reflect.Value
	key_type := rf.Type().Key()
	if key_type.Kind() == reflect.Ptr {
		v = reflect.New(key_type.Elem())
	} else {
		v = reflect.New(key_type)
	}

	state.Push(new_userdata(state, v))
	return 1
}

func map_new_val(state *lua.LState) int {
	m := state.CheckUserData(1)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Map {
		state.RaiseError(fmt.Sprintf("field is %s need map type", rf.Type().Name()))
	}

	var v reflect.Value
	elem_type := rf.Type().Elem()
	if elem_type.Kind() == reflect.Ptr {
		v = reflect.New(elem_type.Elem())
	} else {
		v = reflect.New(elem_type)
	}

	state.Push(new_userdata(state, v))
	return 1
}

func array_new_elem(state *lua.LState) int {
	m := state.CheckUserData(1)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Slice && rf.Kind() != reflect.Array {
		state.RaiseError(fmt.Sprintf("field is %s need slice/array type", rf.Type().Name()))
	}

	var v reflect.Value
	elem_type := rf.Type().Elem()
	if elem_type.Kind() == reflect.Ptr {
		v = reflect.New(elem_type.Elem())
	} else {
		v = reflect.New(elem_type)
	}

	state.Push(new_userdata(state, v))
	return 1
}

func array_foreach(state *lua.LState) int {
	m := state.CheckUserData(1)
	cb := state.CheckFunction(2)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Slice && rf.Kind() != reflect.Array {
		state.RaiseError(fmt.Sprintf("field is %s need slice/array type", rf.Type().Name()))
	}

	for i := 0; i < rf.Len(); i++ {
		v := rf.Index(i)

		state.Push(cb)
		state.Push(lua.LNumber(i))
		state.Push(new_userdata(state, v))
		state.Call(2, 1)

		ret := state.CheckNumber(-1)
		state.Pop(1)

		if ret != 1 {
			break
		}
	}
	return 0
}

func array_get(state *lua.LState) int {
	m := state.CheckUserData(1)
	i := state.CheckNumber(2)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Slice && rf.Kind() != reflect.Array {
		state.RaiseError(fmt.Sprintf("field is %s need slice/array type", rf.Type().Name()))
	}

	v := rf.Index(int(i))
	state.Push(new_userdata(state, v))
	return 1
}

func array_set(state *lua.LState) int {
	m := state.CheckUserData(1)
	i := state.CheckNumber(2)
	new_v := state.CheckUserData(3)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Slice && rf.Kind() != reflect.Array {
		state.RaiseError(fmt.Sprintf("field is %s need slice/array type", rf.Type().Name()))
	}

	v := rf.Index(int(i))
	if vrf, ok := new_v.Value.(reflect.Value); ok {
		v.Set(vrf)
	} else {
		v.Set(reflect.ValueOf(new_v.Value))
	}

	return 0
}

func array_slice(state *lua.LState) int {
	m := state.CheckUserData(1)
	i := state.CheckNumber(2)
	j := state.CheckNumber(3)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Slice && rf.Kind() != reflect.Array {
		state.RaiseError(fmt.Sprintf("field is %s need slice/array type", rf.Type().Name()))
	}

	ret := rf.Slice(int(i), int(j))
	state.Push(new_userdata(state, ret))
	return 1
}

func slice_append(state *lua.LState) int {
	m := state.CheckUserData(1)
	v := state.CheckUserData(2)

	rf, ok := m.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if rf.Kind() != reflect.Slice {
		state.RaiseError(fmt.Sprintf("field is %s need slice type", rf.Type().Name()))
	}

	var new_slice reflect.Value
	if vrf, ok := v.Value.(reflect.Value); ok {
		new_slice = reflect.Append(rf, vrf)
	} else {
		new_slice = reflect.Append(rf, reflect.ValueOf(v.Value))
	}

	state.Push(new_userdata(state, new_slice))
	return 1
}

func new_boolean(state *lua.LState) int {
	val := state.CheckBool(1)
	state.Push(new_userdata(state, bool(val)))
	return 1
}
func new_int(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, int(val)))
	return 1
}
func new_int8(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, int8(val)))
	return 1
}
func new_int16(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, int16(val)))
	return 1
}
func new_int32(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, int32(val)))
	return 1
}
func new_int64(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, int64(val)))
	return 1
}
func new_uint8(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, uint8(val)))
	return 1
}
func new_uint16(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, uint16(val)))
	return 1
}
func new_uint32(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, uint32(val)))
	return 1
}
func new_uint64(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(new_userdata(state, uint64(val)))
	return 1
}
func new_string(state *lua.LState) int {
	val := state.CheckString(1)
	state.Push(new_userdata(state, val))
	return 1
}

func new_with_name(state *lua.LState) int {
	load_types()

	type_name := state.CheckString(1)
	use_ptr := state.CheckBool(2)
	t, ok := types[type_name]
	if !ok {
		state.RaiseError(fmt.Sprintf("type:%s not found", type_name))
	}

	v := reflect.New(t)
	if !use_ptr {
		v = v.Elem()
	}

	state.Push(new_userdata(state, v))
	return 1
}

func new_interface(state *lua.LState) int {
	var tmp []interface{}

	ret := reflect.New(reflect.TypeOf(tmp).Elem()).Elem()
	state.Push(new_userdata(state, ret))
	return 1
}

func search_type_name(state *lua.LState) int {
	load_types()

	include := state.CheckString(1)

	ret := &lua.LTable{}
	for name := range types {
		if include == "" || strings.Index(name, include) >= 0 {
			ret.Append(lua.LString(name))
		}
	}

	state.Push(ret)
	return 1
}

func get_boolean(state *lua.LState) int {
	ud := state.CheckUserData(1)

	var b bool
	switch v := ud.Value.(type) {
	case *bool:
		b = *v

	case bool:
		b = v

	case reflect.Value:
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Bool {
			state.RaiseError(fmt.Sprintf("field is %s need bool type", v.Type().Name()))
		}
		b = v.Bool()

	default:
		state.RaiseError("need bool/reflect.Value")
	}

	state.Push(lua.LBool(b))
	return 1
}

func set_boolean(state *lua.LState) int {
	old_val := state.CheckUserData(1)
	new_val := state.CheckBool(2)

	ro, ok := old_val.Value.(reflect.Value)
	if !ok {
		state.RaiseError("need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}
	if ro.Kind() != reflect.Bool {
		state.RaiseError(fmt.Sprintf("field is %s need boolean type", ro.Type().Name()))
	}

	ro.SetBool(new_val)
	return 0
}

func get_string(state *lua.LState) int {
	ud := state.CheckUserData(1)

	var str string
	switch v := ud.Value.(type) {
	case *string:
		str = *v

	case string:
		str = v

	case reflect.Value:
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.String {
			state.RaiseError(fmt.Sprintf("field is %s need string type", v.Type().Name()))
		}
		str = v.String()

	default:
		state.RaiseError("need string/reflect.Value")
	}

	state.Push(lua.LString(str))
	return 1
}

func set_string(state *lua.LState) int {
	old_val := state.CheckUserData(1)
	new_val := state.CheckString(2)

	ro, ok := old_val.Value.(reflect.Value)
	if !ok {
		state.RaiseError("need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}

	if ro.Kind() != reflect.String {
		state.RaiseError(fmt.Sprintf("field is %s need string type", ro.Type().Name()))
	}

	ro.SetString(new_val)
	return 0
}

func get_number(state *lua.LState) int {
	ud := state.CheckUserData(1)

	switch v := ud.Value.(type) {
	case int:
		state.Push(lua.LNumber(v))
		return 1
	case int8:
		state.Push(lua.LNumber(v))
		return 1
	case int16:
		state.Push(lua.LNumber(v))
		return 1
	case int32:
		state.Push(lua.LNumber(v))
		return 1
	case int64:
		state.Push(lua.LNumber(v))
		return 1
	case uint:
		state.Push(lua.LNumber(v))
		return 1
	case uint8:
		state.Push(lua.LNumber(v))
		return 1
	case uint16:
		state.Push(lua.LNumber(v))
		return 1
	case uint32:
		state.Push(lua.LNumber(v))
		return 1
	case uint64:
		state.Push(lua.LNumber(v))
		return 1
	case float32:
		state.Push(lua.LNumber(v))
		return 1
	case float64:
		state.Push(lua.LNumber(v))
		return 1

	case reflect.Value:
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			state.Push(lua.LNumber(v.Int()))
			return 1
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			state.Push(lua.LNumber(v.Uint()))
			return 1
		case reflect.Float32, reflect.Float64:
			state.Push(lua.LNumber(v.Float()))
			return 1
		default:
			state.RaiseError(fmt.Sprintf("field is %s need number type", v.Type().Name()))
			return 0
		}
	default:
		state.RaiseError("need number/reflect.Value")
		return 0
	}
}

func set_number(state *lua.LState) int {
	old_val := state.CheckUserData(1)
	new_val := state.CheckNumber(2)

	ro, ok := old_val.Value.(reflect.Value)
	if !ok {
		state.RaiseError("need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}
	switch ro.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ro.SetInt(int64(new_val))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		ro.SetUint(uint64(new_val))
	case reflect.Float32, reflect.Float64:
		ro.SetFloat(float64(new_val))
	default:
		state.RaiseError(fmt.Sprintf("field is %s %s need number type", ro.Type(), ro.Kind()))
	}
	return 0
}

func set_any(state *lua.LState) int {
	old_val := state.CheckUserData(1)
	new_val := state.CheckUserData(2)

	ro, ok := old_val.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}

	if rn, ok := new_val.Value.(reflect.Value); ok {
		ro.Set(rn)
	} else {
		ro.Set(reflect.ValueOf(new_val.Value))
	}
	return 0
}

func get_obj_type(state *lua.LState) int {
	ud := state.CheckUserData(1)
	var t reflect.Type
	switch v := ud.Value.(type) {
	case reflect.Value:
		t = v.Type()
	case reflect.Type:
		t = v
	default:
		t = reflect.TypeOf(ud.Value)
	}

	state.Push(new_userdata(state, t))
	return 1
}

func get_type_with_name(state *lua.LState) int {
	load_types()

	type_name := state.CheckString(1)
	ptr := state.CheckBool(2)
	t, ok := types[type_name]
	if !ok {
		state.RaiseError(fmt.Sprintf("type:%s not found", type_name))
	}

	if ptr {
		state.Push(new_userdata(state, reflect.PtrTo(t)))
	} else {
		state.Push(new_userdata(state, t))
	}
	return 1
}

func get_global_with_name(state *lua.LState) int {
	global_name := state.CheckString(1)
	global, err := goof_troop.Global(global_name)
	if !global.IsValid() || err != nil {
		state.RaiseError(fmt.Sprintf("global:%s not found", global_name))
	}

	state.Push(new_userdata(state, global))
	return 1
}

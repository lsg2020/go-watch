package go_watch

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"bou.ke/monkey"
	lua "github.com/yuin/gopher-lua"
	"github.com/zeebo/goof"
)

const moduleName = "go_watch"
const debugCtx = "go_watch_debug_ctx"

var exports map[string]lua.LGFunction
var goofTroop goof.Troop

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
		"map_make":    map_make,

		"array_new_elem": array_new_elem,
		"array_foreach":  array_foreach,
		"array_get":      array_get,
		"array_set":      array_set,
		"array_slice":    array_slice,
		"slice_append":   slice_append,
		"slice_make":     slice_make,

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
	_, err := goofTroop.Global("")
	if err != nil {
		return nil, err
	}

	state := lua.NewState()
	ud := new_userdata(state, &Context{root: root, print: print})
	state.SetGlobal(debugCtx, ud)

	state.PreloadModule(moduleName, func(state *lua.LState) int {
		mod := state.SetFuncs(state.NewTable(), exports)
		state.Push(mod)
		return 1
	})

	return state, nil
}

func Execute(state *lua.LState, script string, session int) error {
	codeTemplate := `
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
	state.Push(lua.LString(codeTemplate))
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
	ud, ok := state.GetGlobal(debugCtx).(*lua.LUserData)
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
var typesMutex sync.Mutex

//go:linkname dwarfName github.com/zeebo/goof.dwarfName
func dwarfName(rtyp reflect.Type) (out string)

func load_types() {
	typesMutex.Lock()
	defer typesMutex.Unlock()

	if types != nil {
		return
	}

	allType, _ := goofTroop.Types()
	types = make(map[string]reflect.Type, len(allType))
	for _, t := range allType {
		types[dwarfName(t)] = t
	}
}

func find_func_with_name(name string) (uintptr, error) {
	rtroop := reflect.ValueOf(goofTroop)
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

	inTypes := make([]reflect.Type, in.Len())
	inValues := make([]reflect.Value, in.Len())
	for i := 1; i <= in.Len(); i++ {
		v := in.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				inTypes[i-1] = r.Type()
				inValues[i-1] = r
			} else {
				inTypes[i-1] = reflect.TypeOf(ud.Value)
				inValues[i-1] = reflect.ValueOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("in params:%d not user data", i))
		}
	}

	outTypes := make([]reflect.Type, out.Len())
	for i := 1; i <= out.Len(); i++ {
		v := out.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				outTypes[i-1] = r.Type()
			} else if t, ok := ud.Value.(reflect.Type); ok {
				outTypes[i-1] = t
			} else {
				outTypes[i-1] = reflect.TypeOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("out params:%d not user data", i))
		}
	}

	newFunc := reflect.MakeFunc(reflect.FuncOf(inTypes, outTypes, false), nil)
	funcPtrVal := reflect.ValueOf(newFunc).FieldByName("ptr").Pointer()
	funcPtr := (*Func)(unsafe.Pointer(funcPtrVal))
	funcPtr.codePtr = ptr

	ret := newFunc.Call(inValues)
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
	name  string
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
		panic(fmt.Errorf("hotfix function error %s %v", ctx.name, err))
		//fmt.Println("hotfix function error", err)
		//goto err_ret
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
	functions, err := goofTroop.Functions()
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
	globals, err := goofTroop.Globals()
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
	newState, _ := NewLuaState(ctx.root, ctx.print)

	fn, err := newState.LoadString(script)
	if err != nil {
		state.RaiseError(fmt.Sprintf("script error:%v", err))
	}

	inTypes := make([]reflect.Type, in.Len())
	for i := 1; i <= in.Len(); i++ {
		v := in.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				inTypes[i-1] = r.Type()
			} else if t, ok := ud.Value.(reflect.Type); ok {
				inTypes[i-1] = t
			} else {
				inTypes[i-1] = reflect.TypeOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("in params:%d not user data", i))
		}
	}

	outTypes := make([]reflect.Type, out.Len())
	for i := 1; i <= out.Len(); i++ {
		v := out.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				outTypes[i-1] = r.Type()
			} else if t, ok := ud.Value.(reflect.Type); ok {
				outTypes[i-1] = t
			} else {
				outTypes[i-1] = reflect.TypeOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("out params:%d not user data", i))
		}
	}

	oldFunc := reflect.MakeFunc(reflect.FuncOf(inTypes, outTypes, false), nil)
	funcPtrVal := reflect.ValueOf(oldFunc).FieldByName("ptr").Pointer()
	funcPtr := (*Func)(unsafe.Pointer(funcPtrVal))
	funcPtr.codePtr = ptr

	hotfix := &HotfixContext{state: newState, fn: fn, in: inTypes, out: outTypes, name: name}
	newFunc := reflect.MakeFunc(reflect.FuncOf(inTypes, outTypes, false), hotfix.Do)
	monkey.Patch(oldFunc.Interface(), newFunc.Interface())
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
	newVal := state.CheckUserData(3)

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
	if rn, ok := newVal.Value.(reflect.Value); ok {
		rf.Set(rn)
	} else {
		rf.Set(reflect.ValueOf(newVal.Value))
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

	var newRs reflect.Value
	rud, ok := ud.Value.(reflect.Value)
	if !ok {
		rud = reflect.ValueOf(ud.Value)
	}

	newRs = reflect.New(rud.Type())
	newRs.Elem().Set(rud)
	state.Push(new_userdata(state, newRs.Elem()))
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
	srcUd := state.CheckUserData(1)
	toUd := state.CheckUserData(2)
	src, ok := srcUd.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}
	if src.Kind() != reflect.Ptr && src.Kind() != reflect.Interface {
		state.RaiseError("param1 need ptr/interface")
	}
	var t reflect.Type
	switch to := toUd.Value.(type) {
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
	keyType := rf.Type().Key()
	if keyType.Kind() == reflect.Ptr {
		v = reflect.New(keyType.Elem())
	} else {
		v = reflect.New(keyType)
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
	elemType := rf.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		v = reflect.New(elemType.Elem())
	} else {
		v = reflect.New(elemType)
	}

	state.Push(new_userdata(state, v))
	return 1
}

func map_make(state *lua.LState) int {
	m := state.CheckUserData(1)

	var mapType reflect.Type
	switch t := m.Value.(type) {
	case reflect.Value:
		mapType = t.Type()
	case reflect.Type:
		mapType = t
	default:
		mapType = reflect.TypeOf(t)
	}
	v := reflect.MakeMap(mapType)
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
	elemType := rf.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		v = reflect.New(elemType.Elem())
	} else {
		v = reflect.New(elemType)
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

	var newSlice reflect.Value
	if vrf, ok := v.Value.(reflect.Value); ok {
		newSlice = reflect.Append(rf, vrf)
	} else {
		newSlice = reflect.Append(rf, reflect.ValueOf(v.Value))
	}

	state.Push(new_userdata(state, newSlice))
	return 1
}

func slice_make(state *lua.LState) int {
	m := state.CheckUserData(1)
	len := state.CheckNumber(2)
	cap := state.CheckNumber(3)

	var sliceType reflect.Type
	switch t := m.Value.(type) {
	case reflect.Value:
		sliceType = t.Type()
	case reflect.Type:
		sliceType = t
	default:
		sliceType = reflect.TypeOf(t)
	}
	v := reflect.MakeSlice(sliceType, int(len), int(cap))
	state.Push(new_userdata(state, v))
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
	var val int64
	lval := state.Get(1)
	if lval.Type() == lua.LTNumber {
		val = int64(state.CheckNumber(1))
	} else if lval.Type() == lua.LTString {
		var err error
		val, err = strconv.ParseInt(state.CheckString(1), 10, 64)
		if err != nil {
			state.RaiseError("param1 parse int error")
		}
	} else {
		state.RaiseError("param1 need number/string")
	}

	state.Push(new_userdata(state, val))
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
	var val uint64
	lval := state.Get(1)
	if lval.Type() == lua.LTNumber {
		val = uint64(state.CheckNumber(1))
	} else if lval.Type() == lua.LTString {
		var err error
		val, err = strconv.ParseUint(state.CheckString(1), 10, 64)
		if err != nil {
			state.RaiseError("param1 parse int error")
		}
	} else {
		state.RaiseError("param1 need number/string")
	}

	state.Push(new_userdata(state, val))
	return 1
}
func new_string(state *lua.LState) int {
	val := state.CheckString(1)
	state.Push(new_userdata(state, val))
	return 1
}

func new_with_name(state *lua.LState) int {
	load_types()

	typeName := state.CheckString(1)
	usePtr := state.CheckBool(2)
	t, ok := types[typeName]
	if !ok {
		state.RaiseError(fmt.Sprintf("type:%s not found", typeName))
	}

	v := reflect.New(t)
	if !usePtr {
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
	oldVal := state.CheckUserData(1)
	newVal := state.CheckBool(2)

	ro, ok := oldVal.Value.(reflect.Value)
	if !ok {
		state.RaiseError("need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}
	if ro.Kind() != reflect.Bool {
		state.RaiseError(fmt.Sprintf("field is %s need boolean type", ro.Type().Name()))
	}

	ro.SetBool(newVal)
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
	oldVal := state.CheckUserData(1)
	newVal := state.CheckString(2)

	ro, ok := oldVal.Value.(reflect.Value)
	if !ok {
		state.RaiseError("need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}

	if ro.Kind() != reflect.String {
		state.RaiseError(fmt.Sprintf("field is %s need string type", ro.Type().Name()))
	}

	ro.SetString(newVal)
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
		state.Push(lua.LString(strconv.FormatInt(v, 10)))
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
		state.Push(lua.LString(strconv.FormatUint(v, 10)))
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
		case reflect.Int64:
			state.Push(lua.LString(strconv.FormatInt(v.Int(), 10)))
			return 1
		case reflect.Uint64:
			state.Push(lua.LString(strconv.FormatUint(v.Uint(), 10)))
			return 1
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
			state.Push(lua.LNumber(v.Int()))
			return 1
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uintptr:
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
	oldVal := state.CheckUserData(1)

	ro, ok := oldVal.Value.(reflect.Value)
	if !ok {
		state.RaiseError("need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}
	switch ro.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		newVal := state.Get(2)
		var intVal int64
		var err error
		if newVal.Type() == lua.LTNumber {
			intVal = int64(state.CheckNumber(2))
		} else if newVal.Type() == lua.LTString {
			intVal, err = strconv.ParseInt(state.CheckString(2), 10, 64)
			if err != nil {
				state.RaiseError("param2 parse int error")
			}
		} else {
			state.RaiseError("param2 need number/string")
		}
		ro.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		newVal := state.Get(2)
		var intVal uint64
		var err error
		if newVal.Type() == lua.LTNumber {
			intVal = uint64(state.CheckNumber(2))
		} else if newVal.Type() == lua.LTString {
			intVal, err = strconv.ParseUint(state.CheckString(2), 10, 64)
			if err != nil {
				state.RaiseError("param2 parse int error")
			}
		} else {
			state.RaiseError("param2 need number/string")
		}
		ro.SetUint(intVal)
	case reflect.Float32, reflect.Float64:
		newVal := state.CheckNumber(2)
		ro.SetFloat(float64(newVal))
	default:
		state.RaiseError(fmt.Sprintf("field is %s %s need number type", ro.Type(), ro.Kind()))
	}
	return 0
}

func set_any(state *lua.LState) int {
	oldVal := state.CheckUserData(1)
	newVal := state.CheckUserData(2)

	ro, ok := oldVal.Value.(reflect.Value)
	if !ok {
		state.RaiseError("param1 need reflect.Value")
	}

	if ro.Kind() == reflect.Ptr {
		ro = ro.Elem()
	}

	if rn, ok := newVal.Value.(reflect.Value); ok {
		ro.Set(rn)
	} else {
		ro.Set(reflect.ValueOf(newVal.Value))
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

	typeName := state.CheckString(1)
	ptr := state.CheckBool(2)
	t, ok := types[typeName]
	if !ok {
		state.RaiseError(fmt.Sprintf("type:%s not found", typeName))
	}

	if ptr {
		state.Push(new_userdata(state, reflect.PtrTo(t)))
	} else {
		state.Push(new_userdata(state, t))
	}
	return 1
}

func get_global_with_name(state *lua.LState) int {
	globalName := state.CheckString(1)
	global, err := goofTroop.Global(globalName)
	if !global.IsValid() || err != nil {
		state.RaiseError(fmt.Sprintf("global:%s not found", globalName))
	}

	state.Push(new_userdata(state, global))
	return 1
}

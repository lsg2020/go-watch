package go_watch

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/lsg2020/gort"
	lua "github.com/yuin/gopher-lua"
)

const moduleName = "go_watch"
const debugCtx = "go_watch_debug_ctx"

var exports map[string]lua.LGFunction

func init() {
	exports = map[string]lua.LGFunction{
		"root_get": lRootGet,
		"print":    lPrint,

		"search_type_name":     lSearchTypeName,
		"search_func_name":     lSearchFuncName,
		"search_global_name":   lSearchGlobalName,
		"get_type_with_name":   lGetTypeWithName,
		"get_obj_type":         lGetObjType,
		"get_global_with_name": lGetGlobalWithName,

		"clone":               lClone,
		"ptr_to_val":          lPtrToVal,
		"val_to_ptr":          lValToPtr,
		"convert_type_to":     lConvertTypeTo,
		"call":                lCall,
		"call_func_with_name": lCallFuncWithName,

		"field_get_by_name": lFieldGetByName,
		"field_set_by_name": lFieldSetByName,

		"map_get":     lMapGet,
		"map_set":     lMapSet,
		"map_del":     lMapDel,
		"map_foreach": lMapForeach,
		"map_new_key": lMapNewKey,
		"map_new_val": lMapNewVal,
		"map_make":    lMapMake,

		"array_new_elem": lArrayNewElem,
		"array_foreach":  lArrayForeach,
		"array_get":      lArrayGet,
		"array_set":      lArraySet,
		"array_slice":    lArraySlice,
		"slice_append":   lSliceAppend,
		"slice_make":     lSliceMake,

		"get_string":  lGetString,
		"set_string":  lSetString,
		"get_number":  lGetNumber,
		"set_number":  lSetNumber,
		"get_boolean": lGetBoolean,
		"set_boolean": lSetBoolean,
		"set_any":     lSetAny,

		"new_boolean":   lNewBoolean,
		"new_int":       lNewInt,
		"new_int8":      lNewInt8,
		"new_int16":     lNewInt16,
		"new_int32":     lNewInt32,
		"new_int64":     lNewInt64,
		"new_uint8":     lNewUint8,
		"new_uint16":    lNewUint16,
		"new_uint32":    lNewUint32,
		"new_uint64":    lNewUint64,
		"new_string":    lNewString,
		"new_with_name": lNewWithName,
		"new_interface": lNewInterface,
	}
}

type RootFunc func(name string) interface{}
type PrintFunc func(session int, str string)
type Context struct {
	root  RootFunc
	print PrintFunc
	dwarf *gort.DwarfRT
}

func NewLuaState(root RootFunc, print PrintFunc) (*lua.LState, error) {
	dwarf, err := gort.NewDwarfRT("")
	if err != nil {
		return nil, err
	}
	return NewLuaStateEx(root, print, dwarf)
}

func NewLuaStateEx(root RootFunc, print PrintFunc, dwarf *gort.DwarfRT) (*lua.LState, error) {
	ctx := &Context{root: root, print: print, dwarf: dwarf}

	state := lua.NewState()
	ud := newUserData(state, ctx)
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

func newUserData(state *lua.LState, data interface{}) *lua.LUserData {
	ud := state.NewUserData()
	ud.Value = data
	return ud
}

func getContext(state *lua.LState) (ctx *Context) {
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

func lRootGet(state *lua.LState) int {
	ctx := getContext(state)
	name := state.CheckString(1)

	ud := newUserData(state, ctx.root(name))
	state.Push(ud)
	return 1
}

func lPrint(state *lua.LState) int {
	ctx := getContext(state)

	session := state.CheckNumber(1)
	str := state.CheckString(2)

	ctx.print(int(session), str)
	return 0
}

func lCall(state *lua.LState) int {
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
		ud := newUserData(state, r)
		state.Push(ud)
	}

	return len(ret)
}

func lCallFuncWithName(state *lua.LState) int {
	ctx := getContext(state)

	name := state.CheckString(1)
	variadic := state.CheckBool(2)
	in := state.CheckTable(3)

	//inTypes := make([]reflect.Type, in.Len())
	inValues := make([]reflect.Value, in.Len())
	for i := 1; i <= in.Len(); i++ {
		v := in.RawGetInt(i)
		if ud, ok := v.(*lua.LUserData); ok {
			if r, ok := ud.Value.(reflect.Value); ok {
				//inTypes[i-1] = r.Type()
				inValues[i-1] = r
			} else {
				//inTypes[i-1] = reflect.TypeOf(ud.Value)
				inValues[i-1] = reflect.ValueOf(ud.Value)
			}
		} else {
			state.RaiseError(fmt.Sprintf("in params:%d not user data", i))
		}
	}

	ret, err := ctx.dwarf.CallFunc(name, variadic, inValues)
	if err != nil {
		state.RaiseError(fmt.Sprintf("call func:%s err:%s", name, err.Error()))
	}
	for _, r := range ret {
		ud := newUserData(state, r)
		state.Push(ud)
	}
	return len(ret)
}

func lSearchFuncName(state *lua.LState) int {
	ctx := getContext(state)
	include := state.CheckString(1)

	ret := &lua.LTable{}
	err := ctx.dwarf.ForeachFunc(func(name string, pc uint64) {
		if include == "" || strings.Index(name, include) >= 0 {
			ret.Append(lua.LString(name))
		}
	})
	if err != nil {
		state.RaiseError(fmt.Sprintf("function search error:%s", err.Error()))
	}

	state.Push(ret)
	return 1
}

func lSearchGlobalName(state *lua.LState) int {
	ctx := getContext(state)
	include := state.CheckString(1)

	ret := &lua.LTable{}
	err := ctx.dwarf.ForeachGlobal(func(name string, v reflect.Value) {
		if include == "" || strings.Index(name, include) >= 0 {
			ret.Append(lua.LString(name))
		}
	})
	if err != nil {
		state.RaiseError(fmt.Sprintf("global search error:%s", err.Error()))
	}

	state.Push(ret)
	return 1
}

func lFieldGetByName(state *lua.LState) int {
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

	ret := newUserData(state, rf)
	state.Push(ret)
	return 1
}

func lFieldSetByName(state *lua.LState) int {
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

func lPtrToVal(state *lua.LState) int {
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

	state.Push(newUserData(state, rf.Elem()))
	return 1
}

func lValToPtr(state *lua.LState) int {
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

	state.Push(newUserData(state, rf.Addr()))
	return 1
}

func lClone(state *lua.LState) int {
	ud := state.CheckUserData(1)
	//ret_ptr := state.CheckBool(2)

	var newRs reflect.Value
	rud, ok := ud.Value.(reflect.Value)
	if !ok {
		rud = reflect.ValueOf(ud.Value)
	}

	newRs = reflect.New(rud.Type())
	newRs.Elem().Set(rud)
	state.Push(newUserData(state, newRs.Elem()))
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
			ret = newUserData(state, new_rs)
		} else {
			ret = newUserData(state, new_rs.Elem())
		}
		state.Push(ret)
		return 1
	*/
}

func lConvertTypeTo(state *lua.LState) int {
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
	state.Push(newUserData(state, ret))
	return 1
}

func lMapGet(state *lua.LState) int {
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
	state.Push(newUserData(state, ret))
	return 1
}

func lMapSet(state *lua.LState) int {
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

func lMapDel(state *lua.LState) int {
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

func lMapForeach(state *lua.LState) int {
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
		state.Push(newUserData(state, k))
		state.Push(newUserData(state, v))
		state.Call(2, 1)

		ret := state.CheckNumber(-1)
		state.Pop(1)

		if ret != 1 {
			break
		}
	}

	return 0
}

func lMapNewKey(state *lua.LState) int {
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

	state.Push(newUserData(state, v))
	return 1
}

func lMapNewVal(state *lua.LState) int {
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

	state.Push(newUserData(state, v))
	return 1
}

func lMapMake(state *lua.LState) int {
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
	state.Push(newUserData(state, v))
	return 1
}

func lArrayNewElem(state *lua.LState) int {
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

	state.Push(newUserData(state, v))
	return 1
}

func lArrayForeach(state *lua.LState) int {
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
		state.Push(newUserData(state, v))
		state.Call(2, 1)

		ret := state.CheckNumber(-1)
		state.Pop(1)

		if ret != 1 {
			break
		}
	}
	return 0
}

func lArrayGet(state *lua.LState) int {
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
	state.Push(newUserData(state, v))
	return 1
}

func lArraySet(state *lua.LState) int {
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

func lArraySlice(state *lua.LState) int {
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
	state.Push(newUserData(state, ret))
	return 1
}

func lSliceAppend(state *lua.LState) int {
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

	state.Push(newUserData(state, newSlice))
	return 1
}

func lSliceMake(state *lua.LState) int {
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
	state.Push(newUserData(state, v))
	return 1
}

func lNewBoolean(state *lua.LState) int {
	val := state.CheckBool(1)
	state.Push(newUserData(state, bool(val)))
	return 1
}
func lNewInt(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(newUserData(state, int(val)))
	return 1
}
func lNewInt8(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(newUserData(state, int8(val)))
	return 1
}
func lNewInt16(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(newUserData(state, int16(val)))
	return 1
}
func lNewInt32(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(newUserData(state, int32(val)))
	return 1
}
func lNewInt64(state *lua.LState) int {
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

	state.Push(newUserData(state, val))
	return 1
}
func lNewUint8(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(newUserData(state, uint8(val)))
	return 1
}
func lNewUint16(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(newUserData(state, uint16(val)))
	return 1
}
func lNewUint32(state *lua.LState) int {
	val := state.CheckNumber(1)
	state.Push(newUserData(state, uint32(val)))
	return 1
}
func lNewUint64(state *lua.LState) int {
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

	state.Push(newUserData(state, val))
	return 1
}
func lNewString(state *lua.LState) int {
	val := state.CheckString(1)
	state.Push(newUserData(state, val))
	return 1
}

func lNewWithName(state *lua.LState) int {
	ctx := getContext(state)

	typeName := state.CheckString(1)
	usePtr := state.CheckBool(2)
	t, err := ctx.dwarf.FindType(typeName)
	if err != nil {
		state.RaiseError(fmt.Sprintf("type:%s not found", typeName))
	}

	v := reflect.New(t)
	if !usePtr {
		v = v.Elem()
	}

	state.Push(newUserData(state, v))
	return 1
}

func lNewInterface(state *lua.LState) int {
	var tmp []interface{}

	ret := reflect.New(reflect.TypeOf(tmp).Elem()).Elem()
	state.Push(newUserData(state, ret))
	return 1
}

func lSearchTypeName(state *lua.LState) int {
	ctx := getContext(state)
	include := state.CheckString(1)

	ret := &lua.LTable{}
	err := ctx.dwarf.ForeachType(func(name string) {
		if include == "" || strings.Index(name, include) >= 0 {
			ret.Append(lua.LString(name))
		}

	})
	if err != nil {
		state.RaiseError(fmt.Sprintf("type search error:%s", err.Error()))
	}

	state.Push(ret)
	return 1
}

func lGetBoolean(state *lua.LState) int {
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

func lSetBoolean(state *lua.LState) int {
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

func lGetString(state *lua.LState) int {
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

func lSetString(state *lua.LState) int {
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

func lGetNumber(state *lua.LState) int {
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

func lSetNumber(state *lua.LState) int {
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

func lSetAny(state *lua.LState) int {
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

func lGetObjType(state *lua.LState) int {
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

	state.Push(newUserData(state, t))
	return 1
}

func lGetTypeWithName(state *lua.LState) int {
	ctx := getContext(state)

	typeName := state.CheckString(1)
	ptr := state.CheckBool(2)
	t, err := ctx.dwarf.FindType(typeName)
	if err != nil {
		state.RaiseError(fmt.Sprintf("type:%s not found", typeName))
	}

	if ptr {
		state.Push(newUserData(state, reflect.PtrTo(t)))
	} else {
		state.Push(newUserData(state, t))
	}
	return 1
}

func lGetGlobalWithName(state *lua.LState) int {
	ctx := getContext(state)
	globalName := state.CheckString(1)
	global, err := ctx.dwarf.FindGlobal(globalName)
	if err != nil || !global.IsValid() {
		state.RaiseError(fmt.Sprintf("global:%s not found", globalName))
	}

	state.Push(newUserData(state, global))
	return 1
}

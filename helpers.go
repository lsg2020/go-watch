package go_watch

import (
	"debug/dwarf"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"github.com/go-delve/delve/pkg/proc"
)

var (
	ErrNotFound = errors.New("not found")
)

//go:linkname findType github.com/go-delve/delve/pkg/proc.(*BinaryInfo).findType
func findType(bi *proc.BinaryInfo, name string) (godwarf.Type, error)

//go:linkname loadModuleData github.com/go-delve/delve/pkg/proc.loadModuleData
func loadModuleData(bi *proc.BinaryInfo, mem proc.MemoryReadWriter) ([]moduleData, error)

//go:linkname imageToModuleData github.com/go-delve/delve/pkg/proc.(*BinaryInfo).imageToModuleData
func imageToModuleData(bi *proc.BinaryInfo, image *proc.Image, mds []moduleData) *moduleData

// delve counterpart to runtime.moduledata
type moduleData struct {
	text, etext   uint64
	types, etypes uint64
	typemapVar    *proc.Variable
}

func dwarfToRuntimeType(bi *proc.BinaryInfo, typ godwarf.Type, name string) (typeAddr uint64, found bool, err error) {
	if typ.Common().Index >= len(bi.Images) {
		return 0, false, ErrNotFound
	}
	so := bi.Images[typ.Common().Index]
	rdr := so.DwarfReader()
	rdr.Seek(typ.Common().Offset)
	e, err := rdr.Next()
	if err != nil {
		return 0, false, err
	}
	entryName, ok := e.Val(dwarf.AttrName).(string)
	if !ok || entryName != name {
		return 0, false, ErrNotFound
	}
	off, ok := e.Val(godwarf.AttrGoRuntimeType).(uint64)
	if !ok || off == 0 {
		return 0, false, ErrNotFound
	}

	mds, err := loadModuleData(bi, &localMemory{})
	if err != nil || len(mds) <= 0 {
		return 0, false, err
	}
	md := imageToModuleData(bi, so, mds)
	if md == nil {
		return 0, false, fmt.Errorf("could not find module data for type %s", typ)
	}

	//if name == "strconv.floatInfo" {
	//	fmt.Printf("-----  %s %s %s %#v\n\n", name, strconv.FormatUint(md.types, 16), strconv.FormatUint(off, 16), e)
	//}
	if off > md.types {
		return off, true, nil
	}
	return md.types + off, true, nil
}

func dwarfTypeName(dtyp dwarf.Type) string {
	// for some reason the debug/dwarf package doesn't set the Name field
	// on the common type for struct fields. what is this misery?
	switch dtyp := dtyp.(type) {
	case *dwarf.StructType:
		return dtyp.StructName
	default:
		return dtyp.Common().Name
	}
}

func entryType(data *dwarf.Data, entry *dwarf.Entry) (dwarf.Type, error) {
	off, ok := entry.Val(dwarf.AttrType).(dwarf.Offset)
	if !ok {
		return nil, fmt.Errorf("unable to find type offset for entry")
	}
	return data.Type(off)
}

func makeInterface(typ, val unsafe.Pointer) interface{} {
	return *(*interface{})(unsafe.Pointer(&[2]unsafe.Pointer{typ, val}))
}

func entryAddress(p uintptr, l int) []byte {
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: p, Len: l, Cap: l}))
}

type Func struct {
	codePtr uintptr
}

type localMemory struct {
}

func (mem *localMemory) ReadMemory(data []byte, addr uint64) (int, error) {
	buf := entryAddress(uintptr(addr), len(data))
	copy(data, buf)
	return len(data), nil
}

func (mem *localMemory) WriteMemory(addr uint64, data []byte) (int, error) {
	buf := entryAddress(uintptr(addr), len(data))
	copy(buf, data)
	return len(data), nil
}

type RootFunc func(name string) interface{}
type PrintFunc func(session int, str string)
type Context struct {
	root    RootFunc
	print   PrintFunc
	bi      *proc.BinaryInfo
	globals map[string]reflect.Value
}

func (ctx *Context) init() error {
	// load debug symbol
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	bi := proc.NewBinaryInfo(runtime.GOOS, runtime.GOARCH)
	err = bi.LoadBinaryInfo(exePath, 0, nil)
	if err != nil {
		return err
	}
	ctx.bi = bi
	ctx.globals = make(map[string]reflect.Value)

	// load variables
	packageVars := reflect.ValueOf(bi).Elem().FieldByName("packageVars")
	if packageVars.IsValid() {
		for i := 0; i < packageVars.Len(); i++ {
			rv := packageVars.Index(i)
			rName := rv.FieldByName("name")
			rAddr := rv.FieldByName("addr")
			rOffset := rv.FieldByName("offset")
			rCU := rv.FieldByName("cu")
			if !rName.IsValid() || !rAddr.IsValid() || !rCU.IsValid() || !rOffset.IsValid() {
				continue
			}
			rImage := rCU.Elem().FieldByName("image")
			if !rImage.IsValid() {
				continue
			}
			rDwarf := rImage.Elem().FieldByName("dwarf")
			if !rDwarf.IsValid() {
				continue
			}
			image := reflect.NewAt(rImage.Type().Elem(), unsafe.Pointer(rImage.Elem().UnsafeAddr())).Interface().(*proc.Image)
			dwarfData := reflect.NewAt(rDwarf.Type().Elem(), unsafe.Pointer(rDwarf.Elem().UnsafeAddr())).Interface().(*dwarf.Data)
			reader := image.DwarfReader()
			reader.Seek(dwarf.Offset(rOffset.Uint()))
			entry, err := reader.Next()
			if err != nil || entry == nil || entry.Tag != dwarf.TagVariable {
				continue
			}
			name, ok := entry.Val(dwarf.AttrName).(string)
			if !ok || rName.String() != name {
				continue
			}

			dtyp, err := entryType(dwarfData, entry)
			if err != nil {
				continue
			}
			dname := dwarfTypeName(dtyp)
			if dname == "<unspecified>" || dname == "" {
				continue
			}

			rtyp, err := ctx.findType(dname)
			if err != nil || rtyp == nil {
				continue
			}
			ctx.globals[name] = reflect.NewAt(rtyp, unsafe.Pointer(uintptr(rAddr.Uint()))).Elem()
		}
	}
	return nil
}

func (ctx *Context) findFunc(name string) (uintptr, error) {
	for _, f := range ctx.bi.Functions {
		if f.Name == name && f.Entry != 0 {
			return uintptr(f.Entry), nil
		}
	}

	return 0, ErrNotFound
}

func (ctx *Context) findType(name string) (reflect.Type, error) {
	dwarfType, err := findType(ctx.bi, name)
	if err != nil {
		return nil, err
	}

	typeAddr, found, err := dwarfToRuntimeType(ctx.bi, dwarfType, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}

	typ := reflect.TypeOf(makeInterface(unsafe.Pointer(uintptr(typeAddr)), nil))
	return typ, nil
}

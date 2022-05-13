package godwarf

import (
	"debug/dwarf"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"github.com/go-delve/delve/pkg/proc"
)

// delve counterpart to runtime.moduledata
type moduleData struct {
	text, etext   uint64
	types, etypes uint64
	typemapVar    *proc.Variable
}

type Func struct {
	codePtr uintptr
}

//go:linkname findType github.com/go-delve/delve/pkg/proc.(*BinaryInfo).findType
func findType(bi *proc.BinaryInfo, name string) (godwarf.Type, error)

//go:linkname loadModuleData github.com/go-delve/delve/pkg/proc.loadModuleData
func loadModuleData(bi *proc.BinaryInfo, mem proc.MemoryReadWriter) ([]moduleData, error)

//go:linkname imageToModuleData github.com/go-delve/delve/pkg/proc.(*BinaryInfo).imageToModuleData
func imageToModuleData(bi *proc.BinaryInfo, image *proc.Image, mds []moduleData) *moduleData

type localMemory int

func (mem *localMemory) ReadMemory(data []byte, addr uint64) (int, error) {
	buf := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: uintptr(addr), Len: len(data), Cap: len(data)}))
	copy(data, buf)
	return len(data), nil
}

func (mem *localMemory) WriteMemory(addr uint64, data []byte) (int, error) {
	return 0, ErrNotSupport
}

func dwarfTypeName(dtyp dwarf.Type) string {
	switch dtyp := dtyp.(type) {
	case *dwarf.StructType:
		return dtyp.StructName
	default:
		name := dtyp.Common().Name
		if name != "" {
			return name
		}
		return dtyp.String()
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

func dwarfToRuntimeType(bi *proc.BinaryInfo, mds []moduleData, typ godwarf.Type, name string) (typeAddr uint64, err error) {
	if typ.Common().Index >= len(bi.Images) {
		return 0, fmt.Errorf("could not find image for type %s", name)
	}
	so := bi.Images[typ.Common().Index]
	rdr := so.DwarfReader()
	rdr.Seek(typ.Common().Offset)
	e, err := rdr.Next()
	if err != nil {
		return 0, fmt.Errorf("could not find dwarf entry for type:%s err:%s", name, err)
	}
	entryName, ok := e.Val(dwarf.AttrName).(string)
	if !ok || entryName != name {
		return 0, fmt.Errorf("could not find name for type:%s entry:%s", name, entryName)
	}
	off, ok := e.Val(godwarf.AttrGoRuntimeType).(uint64)
	if !ok || off == 0 {
		return 0, fmt.Errorf("could not find runtime type for type:%s", name)
	}

	md := imageToModuleData(bi, so, mds)
	if md == nil {
		return 0, fmt.Errorf("could not find module data for type %s", name)
	}

	typeAddr = md.types + off
	if typeAddr < md.types || typeAddr >= md.etypes {
		return off, nil
	}
	return typeAddr, nil
}

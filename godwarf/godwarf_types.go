package godwarf

import (
	"reflect"
	"unsafe"
)

func (d *Dwarf) ForeachType(f func(name string)) error {
	if err := d.check(); err != nil {
		return err
	}

	types, err := d.bi.Types()
	if err != nil {
		return err
	}
	for _, name := range types {
		f(name)
	}
	return nil
}

func (d *Dwarf) FindType(name string) (reflect.Type, error) {
	if err := d.check(); err != nil {
		return nil, err
	}

	dwarfType, err := findType(d.bi, name)
	if err != nil {
		return nil, err
	}

	typeAddr, err := dwarfToRuntimeType(d.bi, d.mds, dwarfType, name)
	if err != nil {
		return nil, err
	}

	typ := reflect.TypeOf(makeInterface(unsafe.Pointer(uintptr(typeAddr)), nil))
	return typ, nil
}

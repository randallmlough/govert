package govert

import (
	"reflect"
)

var (
	primaryKeyString = ""
	primaryKeyInt    = 0
	primaryKeyUint   = uint(0)
)

func primaryKeyType(typ interface{}) reflect.Type {

	return reflect.TypeOf(typ)
}

func underlyingType(typ reflect.Type) string {
	return typ.Kind().String()
}

package govert

import (
	"reflect"
	"strings"
	"text/template"
)

var funcMap = template.FuncMap{
	"isBuiltin":         IsBuiltin,
	"getUnderlyingType": GetUnderlyingType,
	"titleCase":         strings.Title,
}

func IsBuiltin(typ reflect.Type) bool {
	switch typ.String() {
	case "bool",
		"int",
		"int8",
		"int16",
		"int32",
		"int64",
		"uint",
		"uint8",
		"uint16",
		"uint32",
		"uint64",
		"float32",
		"float64",
		"string":
		return true
	}
	return false
}

func GetUnderlyingType(typ reflect.Type) string {
	return typ.Kind().String()
}

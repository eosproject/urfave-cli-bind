package clibind

import (
	"reflect"
	"strings"
	"time"
)

// Takes T from *T if *T value passed
func unreferenceValue(t reflect.Value) reflect.Value {
	for t.Kind() == reflect.Pointer {
		if t.IsNil() {
			return reflect.New(t.Type().Elem()).Elem()
		}
		t = t.Elem()
	}
	return t
}

// Takes T from *T if *T type passed
func unreferenceType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// Returns true whenever passed value is a struct and not supported type (like time.Time{})
func isStructLike(t reflect.Type) bool {
	t = unreferenceType(t)
	return t.Kind() == reflect.Struct && t != reflect.TypeOf(time.Time{})
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isAnyInt(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	default:
		return false
	}
}
func isAnyUint(k reflect.Kind) bool {
	switch k {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	default:
		return false
	}
}

func castAndSetInt(field reflect.Value, v int64) {
	switch field.Kind() {
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		field.SetInt(v)
	default:
		// pointer?
		if field.Kind() == reflect.Pointer && isAnyInt(field.Elem().Kind()) {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field.Elem().SetInt(v)
		}
	}
}
func castAndSetUint(field reflect.Value, v uint64) {
	switch field.Kind() {
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uintptr:
		field.SetUint(v)
	default:
		if field.Kind() == reflect.Pointer && isAnyUint(field.Elem().Kind()) {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field.Elem().SetUint(v)
		}
	}
}

// parseNamesWithOptions supports ",omitempty" as an extra token.
func parseNamesWithOptions(tag string) (name string, aliases []string, omitEmpty bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", nil, false
	}
	parts := splitCSV(tag)
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, false
	}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		switch p {
		case "omitempty":
			omitEmpty = true
		default:
			aliases = append(aliases, p)
		}
	}
	return parts[0], aliases, omitEmpty
}

package clibind

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/urfave/cli/v3"
)

// FlagsFromStruct inspects exported fields with `cli` and other tags and generates cli.Flag definitions.
// It is safe to pass either a struct or a pointer to a struct. Unexported fields are ignored.
func FlagsFromStruct(v any) []cli.Flag {
	rt := unreferenceType(reflect.TypeOf(v))
	if rt.Kind() != reflect.Struct {
		return nil
	}
	var flags []cli.Flag
	genFlagsForStruct(rt, "", &flags) // empty prefix at root
	return flags
}

func genFlagsForStruct(rt reflect.Type, inheritedPrefix string, out *[]cli.Flag) {
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if sf.PkgPath != "" { // unexported
			continue
		}

		// If this is a (sub)struct with cliPrefix, recurse
		if isStructLike(sf.Type) && sf.Tag.Get(tagCLIPrefix) != "" {
			pfx := inheritedPrefix + sf.Tag.Get(tagCLIPrefix)
			genFlagsForStruct(unreferenceType(sf.Type), pfx, out)
			continue
		}

		// Regular field with cli tag
		name, aliases, omitEmpty := parseNamesWithOptions(sf.Tag.Get(tagCLI))
		if name == "" {
			name = strings.ToLower(sf.Name)
			// still allow anonymous embedded structs (without cliPrefix) to be flattened
			if sf.Anonymous && isStructLike(sf.Type) {
				genFlagsForStruct(unreferenceType(sf.Type), inheritedPrefix, out)
				continue
			}
		}

		// apply inherited prefix to the primary name and aliases
		name = inheritedPrefix + name
		for i := range aliases {
			if len(aliases[i]) > 1 {
				aliases[i] = inheritedPrefix + aliases[i]
			}
		}

		usage := sf.Tag.Get(tagCLIUsage)
		def := sf.Tag.Get(tagCLIDefault)
		// before your switch:
		ft := sf.Type
		kind := unreferenceType(ft).Kind()

		required := !omitEmpty && def == ""

		switch {
		case ft == reflect.TypeOf(time.Second):
			*out = append(*out, &cli.StringFlag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				Value:       def,
				DefaultText: def,
				Required:    required,
			})
		case kind == reflect.Bool:
			f, _ := strconv.ParseBool(def)
			*out = append(*out, &cli.BoolFlag{
				Name:     name,
				Aliases:  aliases,
				Usage:    usage,
				Value:    f,
				Required: required,
			})
		case isAnyInt(kind):
			f, _ := strconv.ParseInt(def, 10, 64)
			*out = append(*out, &cli.Int64Flag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				Value:       f,
				DefaultText: def,
				Required:    required,
			})
		case isAnyUint(kind):
			f, _ := strconv.ParseUint(def, 10, 64)
			*out = append(*out, &cli.Uint64Flag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				Value:       f,
				DefaultText: def,
				Required:    required,
			})
		case kind == reflect.Float32 || kind == reflect.Float64:
			f, _ := strconv.ParseFloat(def, 64)
			*out = append(*out, &cli.Float64Flag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				Value:       f,
				DefaultText: def,
				Required:    required,
			})

		case ft == reflect.TypeOf(time.Time{}):
			tf := &cli.StringFlag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				DefaultText: def,

				Value:    def,
				Required: required,
			}
			*out = append(*out, tf)

		case ft == reflect.TypeOf(uuid.UUID{}):
			*out = append(*out, &cli.StringFlag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				Value:       def,
				DefaultText: def,
				Required:    required,
			})
		case kind == reflect.String:
			*out = append(*out, &cli.StringFlag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				Value:       def,
				DefaultText: def,
				Required:    required,
			})
		case kind == reflect.Slice:
			*out = append(*out, &cli.StringSliceFlag{
				Name:        name,
				Aliases:     aliases,
				Usage:       usage,
				Value:       splitCSV(def),
				DefaultText: def,
				Required:    required,
			})
		}

	}
}

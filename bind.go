// Package clibind provides a tag/reflect-based mapper between urfave/cli/v2 flags and a struct.
// Usage:
//
//	type Config struct {
//	    Name   string        `cli:"name,n,A"  cliDefault:"guest" cliUsage:"User name"`
//	    Count  int           `cli:"count,c"   cliDefault:"3"     cliUsage:"How many items"`
//	    Delay  time.Duration `cli:"delay,d"   cliDefault:"250ms" cliUsage:"Wait duration"`
//	    When   time.Time     `cli:"when,w"    cliDefault:"2025-01-02T15:04:05Z" cliUsage:"When (RFC3339)"`
//	    IDs    []uuid.UUID   `cli:"ids"       cliDefault:"b33a...-0001, b33a...-0002" cliUsage:"Comma-separated UUIDs"`
//	    Tags   []string      `cli:"tags"      cliDefault:"alpha,beta" cliUsage:"Comma-separated"`
//	}
//
//	app := &cli.App{
//	    Flags: clibind.FlagsFromStruct(Config{}),
//	    Action: func(ctx context.Context, c *cli.Command) error {
//	        var cfg Config
//	        if err := clibind.Bind(c, &cfg); err != nil { return err }
//	        // use cfg...
//	        return nil
//	    },
//	}
package clibind

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/urfave/cli/v3"
)

const (
	tagCLI         = "cli"           // "name,alias,Short"
	tagCLIDefault  = "cliDefault"    // default value as string
	tagCLIUsage    = "cliUsage"      // usage/help string
	tagCLITimeFmt  = "cliTimeLayout" // optional time layout (default RFC3339)
	tagCLIPrefix   = "cliPrefix"
	defaultTimeFmt = time.RFC3339
)

// Bind populates struct fields from CLI flag values defined in the given
// command context. It expects dest to be a pointer to a struct whose fields
// are tagged or named to correspond to the command’s flags.
//
// dest must be a non-nil pointer to a struct, otherwise Bind returns an error.
func Bind(ctx *cli.Command, dest any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("Bind: dest must be a non-nil pointer to a struct")
	}
	v, err := bindStruct(ctx, unreferenceType(rv.Type()), "")
	if err != nil {
		return err
	}
	if v != nil {
		rv.Elem().Set(*v)
	}
	return nil
}

func bindStruct(ctx *cli.Command, t reflect.Type, prefix string) (vp *reflect.Value, err error) {
	t = unreferenceType(t)

	v := reflect.New(t).Elem()
	defined := false

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		fv := v.Field(i)

		name, _, omitEmpty := parseNamesWithOptions(sf.Tag.Get(tagCLI))
		if name == "" {
			name = strings.ToLower(sf.Name)
		}
		name = prefix + name

		if isStructLike(sf.Type) {
			pfx := prefix
			if !sf.Anonymous {
				pfx += sf.Tag.Get(tagCLIPrefix)
			} else if sf.Tag.Get(tagCLI) != "" {
				return nil, fmt.Errorf("embedded struct %s has cli tag, but unsupported", sf.Name)
			}

			subv, err := bindStruct(ctx, sf.Type, pfx)
			if err != nil {
				return nil, fmt.Errorf("bind substruct %s: %w", sf.Name, err)
			}
			if subv != nil {
				defined = true
				fv.Set(*subv)
			}
			continue
		}

		if !ctx.IsSet(name) && omitEmpty {
			continue
		}
		if err := setFieldValue(ctx, name, sf, unreferenceValue(fv)); err != nil {
			return nil, fmt.Errorf("set field %s value: %w", sf.Name, err)
		}
		defined = true
		log.Printf("set value to %s: %v", name, fv.String())
	}
	if defined {
		return &v, nil
	}
	return nil, nil
}

// setFieldValue reads a CLI flag and sets the corresponding struct field.
func setFieldValue(ctx *cli.Command, name string, sf reflect.StructField, field reflect.Value) error {
	log.Printf("field %s value %v", name, ctx.String(name))

	t := unreferenceType(sf.Type)

	switch {
	case t == reflect.TypeOf(time.Second):
		s := ctx.String(name)
		if s == "" {
			field.Set(reflect.ValueOf(time.Duration(0)))
			return nil
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		field.Set(reflect.ValueOf(d))

	case t.Kind() == reflect.Bool:
		field.SetBool(ctx.Bool(name))

	case isAnyInt(t.Kind()):
		castAndSetInt(field, ctx.Int64(name))

	case isAnyUint(t.Kind()):
		castAndSetUint(field, ctx.Uint64(name))

	case t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64:
		field.SetFloat(ctx.Float64(name))

	case t == reflect.TypeOf(time.Time{}):
		timeLayout := sf.Tag.Get(tagCLITimeFmt)
		if timeLayout == "" {
			timeLayout = defaultTimeFmt
		}
		s := ctx.String(name)
		if s == "" {
			field.Set(reflect.ValueOf(time.Time{}))
			return nil
		}
		t, err := time.Parse(timeLayout, s)
		if err != nil {
			return fmt.Errorf("time parse: %w", err)
		}
		field.Set(reflect.ValueOf(t))

	case t == reflect.TypeOf(uuid.UUID{}):
		s := ctx.String(name)
		if s == "" {
			field.Set(reflect.ValueOf(uuid.Nil))
			return nil
		}
		id, err := uuid.FromString(s)
		if err != nil {
			return fmt.Errorf("parse uuid: %w", err)
		}
		field.Set(reflect.ValueOf(id))

	case t.Kind() == reflect.String:
		field.SetString(ctx.String(name))

	case t.Kind() == reflect.Slice:
		return setSliceField(ctx, name, sf, field)
	}
	return nil
}

// setSliceField handles slice types (string, int, uuid, etc.)
func setSliceField(ctx *cli.Command, name string, sf reflect.StructField, field reflect.Value) error {
	raw := ctx.StringSlice(name)
	if len(raw) == 0 {
		return nil
	}

	ft := sf.Type
	t := ft.Elem()
	out := reflect.MakeSlice(reflect.SliceOf(t), 0, len(raw))

	for _, s := range raw {
		val := reflect.New(t).Elem()

		switch {
		case t == reflect.TypeOf(time.Second):
			if s == "" {
				val.Set(reflect.ValueOf(time.Duration(0)))
				continue
			}
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("parse duration: %w", err)
			}
			val.Set(reflect.ValueOf(d))

		case t.Kind() == reflect.Bool:
			tr, _ := strconv.ParseBool(s)
			val.SetBool(tr)

		case isAnyInt(t.Kind()):
			i, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return fmt.Errorf("parse int: %w", err)
			}
			castAndSetInt(val, i)

		case isAnyUint(t.Kind()):
			i, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return fmt.Errorf("parse uint: %w", err)
			}
			castAndSetUint(val, i)

		case t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64:
			i, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return fmt.Errorf("parse float: %w", err)
			}
			val.SetFloat(i)

		case t == reflect.TypeOf(time.Time{}):
			timeLayout := sf.Tag.Get(tagCLITimeFmt)
			if timeLayout == "" {
				timeLayout = defaultTimeFmt
			}
			if s == "" {
				val.Set(reflect.ValueOf(time.Time{}))
				continue
			}
			t, err := time.Parse(timeLayout, s)
			if err != nil {
				return fmt.Errorf("time parse: %w", err)
			}
			val.Set(reflect.ValueOf(t))

		case t == reflect.TypeOf(uuid.UUID{}):
			if s == "" {
				val.Set(reflect.ValueOf(uuid.Nil))
				continue
			}
			id, err := uuid.FromString(s)
			if err != nil {
				return fmt.Errorf("parse uuid: %w", err)
			}
			val.Set(reflect.ValueOf(id))

		case t.Kind() == reflect.String:
			val.SetString(s)

		case t.Kind() == reflect.Slice:
			return fmt.Errorf("matrix type at %s is not supported", sf.Name)
		}

		out = reflect.Append(out, val)
	}
	field.Set(out)
	return nil
}

// WithBinding wraps a typed handler function so that it automatically binds CLI
// flag values to a struct before invoking the handler.
//
// The generic parameter T defines the type of the struct into which CLI flags
// will be bound. The provided function fn receives a populated instance of T.
//
// This allows you to write clean, strongly typed handlers without manually
// parsing or binding CLI flags.
func WithBinding[T any](
	fn func(ctx context.Context, t T) error,
) func(ctx context.Context, c *cli.Command) (err error) {
	return func(ctx context.Context, c *cli.Command) (err error) {
		var t T
		if err = Bind(c, &t); err != nil {
			return fmt.Errorf("bind flags: %w", err)
		}
		return fn(ctx, t)
	}
}

// CommandWithBinding creates a new CLI command that automatically binds
// command-line flags into a typed configuration struct before executing
// the provided handler function.
//
// It combines command construction and type-safe binding in one step.
//
// If base is nil, a new *cli.Command is created. The resulting command’s
// Action is set using WithBinding(fn), and its Name is set to the provided
// name.
//
// Example:
//
//	func runServer(ctx context.Context, cfg ServerConfig) error { ... }
//
//	root := &cli.Command{Name: "root"}
//	server := clibind.CommandWithBinding(root, "serve", runServer)
//
// When executed, the "serve" subcommand will parse CLI flags, populate
// a ServerConfig instance via Bind, and then invoke runServer with the
// bound configuration.
func CommandWithBinding[T any](
	base *cli.Command,
	name string,
	fn func(ctx context.Context, t T) error,
) *cli.Command {
	if base == nil {
		base = &cli.Command{}
	}
	base.Action = WithBinding(fn)
	base.Name = name
	return base
}

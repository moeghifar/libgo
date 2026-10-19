// Package envy loads environment variables, and an optional .env file, into a tagged struct.
package envy

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Load fills target from `default:` tags, then the .env file (if available)
// and the environment. A `required:"true"` field left empty is an error.
func Load(target any) error {
	if err := Defaults(target); err != nil {
		return err
	}
	if err := loadEnvFile(); err != nil {
		return err
	}
	return parse(target, os.Getenv)
}

// Defaults applies only the `default:` tags, reading neither the environment
// nor a .env file, and ignores `required`.
func Defaults(target any) error { return parse(target, nil) }

// parse fills v from getenv, falling back to the default tag. A nil getenv
// applies defaults only.
func parse(v any, getenv func(string) string) error {
	ptrVal := reflect.ValueOf(v)
	if ptrVal.Kind() != reflect.Pointer || ptrVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("target must be a pointer to a struct")
	}

	val := ptrVal.Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		structField := typ.Field(i)

		// Handle nested structs (recursive)
		if field.Kind() == reflect.Struct {
			if err := parse(field.Addr().Interface(), getenv); err != nil {
				return err
			}
			continue
		}

		// Get tags
		envKey := structField.Tag.Get("env")
		defaultValue := structField.Tag.Get("default")
		required := structField.Tag.Get("required")

		if envKey == "" {
			continue // Skip fields without env tag
		}

		var envVal string
		if getenv != nil {
			envVal = getenv(envKey)
		}

		// Use default if empty
		if envVal == "" {
			if getenv != nil && required == "true" && defaultValue != "" {
				slog.Default().Warn("required env var not set, using default", "var", envKey, "default", defaultValue)
			}
			envVal = defaultValue
		}

		// Check required
		if getenv != nil && envVal == "" && required == "true" {
			return fmt.Errorf("var `%s` is required", envKey)
		}

		// Set value based on type
		if envVal != "" {
			if err := setField(field, envVal, structField.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func setField(field reflect.Value, value string, fieldName string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intValue, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int for field %s: %w", fieldName, err)
		}
		field.SetInt(intValue)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintValue, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid uint for field %s: %w", fieldName, err)
		}
		field.SetUint(uintValue)
	case reflect.Bool:
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool for field %s: %w", fieldName, err)
		}
		field.SetBool(boolValue)
	case reflect.Float32, reflect.Float64:
		floatValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid float for field %s: %w", fieldName, err)
		}
		field.SetFloat(floatValue)
	case reflect.Slice:
		return setSlice(field, value, fieldName)
	default:
		return fmt.Errorf("unsupported type: %v for field %s", field.Kind(), fieldName)
	}
	return nil
}

func setSlice(field reflect.Value, value string, fieldName string) error {
	parts := strings.Split(value, ",")
	// Trim spaces from each part
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Create a new slice with the correct length
	slice := reflect.MakeSlice(field.Type(), 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue // Skip empty parts if desired, or handle them?
			// For logic like "A,,B", splits to ["A", "", "B"].
			// Usually valid elements are expected. Let's skip empty strings for now.
		}

		elemVal := reflect.New(field.Type().Elem()).Elem()

		switch elemVal.Kind() {
		case reflect.String:
			elemVal.SetString(part)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid int in slice for field %s: %w", fieldName, err)
			}
			elemVal.SetInt(val)
		case reflect.Float32, reflect.Float64:
			val, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return fmt.Errorf("invalid float in slice for field %s: %w", fieldName, err)
			}
			elemVal.SetFloat(val)
		default:
			return fmt.Errorf("unsupported slice element type: %v for field %s", elemVal.Kind(), fieldName)
		}
		slice = reflect.Append(slice, elemVal)
	}

	field.Set(slice)
	return nil
}

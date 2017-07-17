package coreutil

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-yaml/yaml"
)

// ParseConfig, reads yaml or json file into a struct
func ParseConfig(srcfile string, out interface{}) (err error) {
	if srcfile == "" {
		return
	}
	f, err := os.Open(srcfile)
	if err != nil {
		return err
	}
	defer f.Close()

	if strings.HasSuffix(srcfile, ".json") {
		// Read as JSON
		json.NewDecoder(f).Decode(out)
		return
	}
	if strings.HasSuffix(srcfile, ".yaml") {
		data, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		// Read as yaml
		yaml.Unmarshal(data, out)
	}
	return err
}

func SaveConfig(name string, obj interface{}) (err error) {
	var data []byte
	if strings.HasSuffix(name, ".json") {
		data, err = json.MarshalIndent(obj, "  ", "  ")
		if err != nil {
			return err
		}
	}
	if strings.HasSuffix(name, ".yaml") {
		data, err = yaml.Marshal(obj)
		if err != nil {
			return err
		}
	}

	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to save config: %v\n", err)
	}
	defer f.Close()

	f.Write(data)
	f.Sync()

	return err
}

// ParseOptions parses mount options like -o uid=100,gid=100 to struct
func ParseOptions(opt string, out interface{}) (err error) {
	mountopts := map[string]string{}
	parts := strings.Split(opt, ",")
	// First Map to keyvalue
	for _, v := range parts {
		if keyindex := strings.Index(v, "="); keyindex != -1 { // Eq
			key := strings.TrimSpace(v[:keyindex])
			value := strings.TrimSpace(v[keyindex+1:])
			mountopts[key] = value
		} else {
			mountopts[v] = "true"
		}
	}

	// Assign map to object by Tag by iterating fields
	typ := reflect.TypeOf(out).Elem() // Should be pointer
	val := reflect.ValueOf(out).Elem()
	for i := 0; i < typ.NumField(); i++ {
		fieldTyp := typ.Field(i)
		fieldVal := val.Field(i)
		name := strings.ToLower(fieldTyp.Name)
		if tag, ok := fieldTyp.Tag.Lookup("opt"); ok {
			tagParts := strings.Split(tag, ",")
			if len(tagParts) > 0 && tagParts[0] != "" {
				name = tagParts[0]
			}
		}

		if v, ok := mountopts[name]; ok {
			err = StringAssign(v, fieldVal.Addr().Interface())
			if err != nil {
				return err
			}
		}
	}
	return
}

// StringAssign parseString and place value in
func StringAssign(s string, v interface{}) (err error) {
	val := reflect.ValueOf(v).Elem()
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64: // More values
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		sval := reflect.ValueOf(parsed)
		val.Set(sval.Convert(val.Type()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64: // More values
		parsed, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		sval := reflect.ValueOf(parsed)
		val.Set(sval.Convert(val.Type()))
	case reflect.Bool:
		parsed, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		sval := reflect.ValueOf(parsed)
		val.Set(sval)
	}

	return
}

func OptionString(o interface{}) string {
	ret := ""
	typ := reflect.TypeOf(o) // Should be pointer
	val := reflect.ValueOf(o)
	for i := 0; i < typ.NumField(); i++ {
		fieldTyp := typ.Field(i)
		fieldVal := val.Field(i)
		name := strings.ToLower(fieldTyp.Name)
		desc := ""
		if tag, ok := fieldTyp.Tag.Lookup("opt"); ok {
			tagParts := strings.Split(tag, ",")
			if len(tagParts) > 0 && tagParts[0] != "" {
				name = tagParts[0]
			}
			/*if len(tagParts) >= 2 {
				desc = tagParts[1]
			}*/
		}
		if i != 0 {
			ret += ","
		}
		if desc != "" {
			ret += fmt.Sprintf("%s=%v (%s)", name, fieldVal.Interface(), desc)
		} else {
			ret += fmt.Sprintf("%s=%v", name, fieldVal.Interface())
		}
	}

	return ret
}
func OptionMap(o interface{}) map[string]string {
	ret := map[string]string{}
	typ := reflect.TypeOf(o) // Should be pointer
	val := reflect.ValueOf(o)
	for i := 0; i < typ.NumField(); i++ {
		fieldTyp := typ.Field(i)
		fieldVal := val.Field(i)
		name := strings.ToLower(fieldTyp.Name)
		if tag, ok := fieldTyp.Tag.Lookup("opt"); ok {
			tagParts := strings.Split(tag, ",")
			if len(tagParts) > 0 && tagParts[0] != "" {
				name = tagParts[0]
			}
		}
		ret[name] = fmt.Sprintf("%v", fieldVal.Interface())
	}
	return ret
}

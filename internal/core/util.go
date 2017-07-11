package core

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/go-yaml/yaml"
)

// Some utils
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

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

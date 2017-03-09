/*-
 * Copyright 2015 Grammarly, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package template

import (
	"encoding/json"
	"fmt"
	"github.com/grammarly/rocker/src/imagename"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/go-yaml/yaml"

	log "github.com/Sirupsen/logrus"
)

// Vars describes the data structure of the build variables
type Vars map[string]interface{}

// Convert map with interface{} key to map with string key if possible
func fixMap(m map[interface{}]interface{}) map[string]interface{} {
	res := make(map[string]interface{})

	for k, v := range m {
		res[k.(string)] = v
	}
	return res
}

func deepMergeMaps(x1, x2 map[string]interface{}) map[string]interface{} {
	for k, v2 := range x2 {
		if v1, ok := x1[k]; ok {
			x1[k] = deepMerge(v1, v2)
		} else {
			x1[k] = v2
		}
	}
	return x1
}

// Deep merge two values
// Going deep only if values are maps
func deepMerge(x1, x2 interface{}) interface{} {
	if x1 == nil {
		return x2
	} else if x2 == nil {
		return x1
	}

	rv1 := reflect.ValueOf(x1)
	rv2 := reflect.ValueOf(x2)

	// We want to merge slices of the same type by appending them to each other
	// instead of overwriting
	if rv1.Kind() == reflect.Slice && rv2.Kind() == reflect.Slice && rv1.Type() == rv2.Type() {
		return reflect.AppendSlice(rv1, rv2).Interface()
	} else if rv2.Kind() == reflect.Map {
		var sm1,sm2 map[string]interface{}
		if rv1.Type() != rv2.Type() {
			log.Fatal("Incompatible map types, can't merge")
		}

		switch x2:= x2.(type) {
		case Vars:
			sm2 = x2
		case map[string]interface{}:
			sm2 = x2
		case map[interface{}]interface{}:
			sm2 = fixMap(x2)
		default:
			log.Fatalf("Unsupported map type: %T", x2)
		}

		switch x1:= x1.(type) {
		case Vars:
			sm1 = x1
		case map[string]interface{}:
			sm1 = x1
		case map[interface{}]interface{}:
			sm1 = fixMap(x1)
		default:
			log.Fatalf("Unsupported map type: %T", x1)
		}
		return deepMergeMaps(sm1, sm2)
	} else {
		return x2
	}
}

// Merge the current Vars structure with the list of other Vars structs
func (vars Vars) Merge(varsList ...Vars) Vars {
	for _, mergeWith := range varsList {
		deepMerge(vars, mergeWith)
	}
	return vars
}

// IsSet returns true if the given key is set
func (vars Vars) IsSet(key string) bool {
	_, ok := vars[key]
	return ok
}

// ToStrings converts Vars to a slice of strings line []string{"KEY=VALUE"}
func (vars Vars) ToStrings() (result []string) {
	for k, v := range vars {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(result)
	return result
}

// ToMapOfInterface casts Vars to map[string]interface{}
func (vars Vars) ToMapOfInterface() map[string]interface{} {
	result := map[string]interface{}{}
	for k, v := range vars {
		result[k] = v
	}
	return result
}

// MarshalJSON serialize Vars to JSON
func (vars Vars) MarshalJSON() ([]byte, error) {
	return json.Marshal(vars.ToStrings())
}

// UnmarshalJSON unserialize Vars from JSON string
func (vars *Vars) UnmarshalJSON(data []byte) (err error) {
	// try unmarshal map to keep backward compatibility
	maps := map[string]interface{}{}
	if err = json.Unmarshal(data, &maps); err == nil {
		*vars = (Vars)(maps)
		return nil
	}
	// unmarshal slice of strings
	strings := []string{}
	if err = json.Unmarshal(data, &strings); err != nil {
		return err
	}
	if *vars, err = VarsFromStrings(strings); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML parses YAML string and returns Vars
func (vars *Vars) UnmarshalYAML(unmarshal func(interface{}) error) (err error) {
	// try unmarshal RockerArtifacts type
	var artifacts imagename.Artifacts
	if err = unmarshal(&artifacts); err != nil {
		return err
	}

	var value map[string]interface{}
	if err = unmarshal(&value); err != nil {
		return err
	}

	// Fill artifacts if present
	if len(artifacts.RockerArtifacts) > 0 {
		value["RockerArtifacts"] = artifacts.RockerArtifacts
	}

	*vars = value

	return nil
}

// VarsFromStrings parses Vars through ParseKvPairs and then loads content from files
// for vars values with "@" prefix
func VarsFromStrings(pairs []string) (vars Vars, err error) {
	vars = ParseKvPairs(pairs)
	for k, v := range vars {
		// We care only about strings
		switch v := v.(type) {
		case string:
			// Read variable content from a file if "@" prefix is given
			if strings.HasPrefix(v, "@") {
				f := v[1:]
				if vars[k], err = loadFileContent(f); err != nil {
					return vars, fmt.Errorf("Failed to read file '%s' for variable %s, error: %s", f, k, err)
				}
			}
			// Unescape "\@"
			if strings.HasPrefix(v, "\\@") {
				vars[k] = v[1:]
			}
		}
	}
	return vars, nil
}

// VarsFromFile reads variables from either JSON or YAML file
func VarsFromFile(filename string) (vars Vars, err error) {
	log.Debugf("Load vars from file %s", filename)

	if filename, err = resolveFileName(filename); err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	vars = Vars{}

	switch filepath.Ext(filename) {
	case ".yaml", ".yml", ".":
		if err := yaml.Unmarshal(data, &vars); err != nil {
			return nil, err
		}
	case ".json":
		if err := json.Unmarshal(data, &vars); err != nil {
			return nil, err
		}
	}

	return vars, nil
}

// VarsFromFileMulti reads multiple files and merge vars
func VarsFromFileMulti(files []string) (Vars, error) {
	var (
		varsList = []Vars{}
		matches  []string
		vars     Vars
		err      error
	)

	for _, pat := range files {
		matches = []string{pat}

		if containsWildcards(pat) {
			if matches, err = filepath.Glob(pat); err != nil {
				return nil, err
			}
		}

		for _, f := range matches {
			if vars, err = VarsFromFile(f); err != nil {
				return nil, err
			}
			varsList = append(varsList, vars)
		}
	}

	return Vars{}.Merge(varsList...), nil
}

// ParseKvPairs parses Vars from a slice of strings e.g. []string{"KEY=VALUE"}
func ParseKvPairs(pairs []string) (vars Vars) {
	vars = make(Vars)
	for _, varPair := range pairs {
		tmp := strings.SplitN(varPair, "=", 2)
		vars[tmp[0]] = tmp[1]
	}
	return vars
}

func loadFileContent(f string) (content string, err error) {
	if f, err = resolveFileName(f); err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func resolveFileName(f string) (string, error) {
	if f == "~" || strings.HasPrefix(f, "~/") {
		f = strings.Replace(f, "~", os.Getenv("HOME"), 1)
	}
	if !filepath.IsAbs(f) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		f = path.Join(wd, f)
	}
	return f, nil
}

// Code borrowed from https://github.com/docker/docker/blob/df0e0c76831bed08cf5e08ac9a1abebf6739da23/builder/support.go
var (
	// `\\\\+|[^\\]|\b|\A` - match any number of "\\" (ie, properly-escaped backslashes), or a single non-backslash character, or a word boundary, or beginning-of-line
	// `\$` - match literal $
	// `[[:alnum:]_]+` - match things like `$SOME_VAR`
	// `{[[:alnum:]_]+}` - match things like `${SOME_VAR}`
	tokenVarsInterpolation = regexp.MustCompile(`(\\|\\\\+|[^\\]|\b|\A)\$([[:alnum:]_]+|{[[:alnum:]_]+})`)
	// this intentionally punts on more exotic interpolations like ${SOME_VAR%suffix} and lets the shell handle those directly
)

// ReplaceString handle vars replacement
func (vars Vars) ReplaceString(str string) string {
	for _, match := range tokenVarsInterpolation.FindAllString(str, -1) {
		idx := strings.Index(match, "\\$")
		if idx != -1 {
			if idx+2 >= len(match) {
				str = strings.Replace(str, match, "\\$", -1)
				continue
			}

			prefix := match[:idx]
			stripped := match[idx+2:]
			str = strings.Replace(str, match, prefix+"$"+stripped, -1)
			continue
		}

		match = match[strings.Index(match, "$"):]
		matchKey := strings.Trim(match, "${}")

		if val, ok := vars[matchKey].(string); ok {
			str = strings.Replace(str, match, val, -1)
		}
	}

	return str
}

func containsWildcards(name string) bool {
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '\\' {
			i++
		} else if ch == '*' || ch == '?' || ch == '[' {
			return true
		}
	}
	return false
}

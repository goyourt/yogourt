package binary

import (
	"fmt"
	"plugin"
)

func LoadPlugin(path string) (*plugin.Plugin, error) {
	plg, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open plugin: %v", err)
	}

	return plg, nil
}

func LoadFunctions(path string, names []string) (map[string]interface{}, error) {
	plg, err := LoadPlugin(path)
	if err != nil {
		return nil, err
	}

	functions := make(map[string]interface{})
	for _, name := range names {
		if function, err := plg.Lookup(name); err == nil {
			functions[name] = function
		}
	}

	return functions, nil
}

package main

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

func ParseFile(filePath string) (Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Unknown keys are errors, not ignored: a misspelled key would silently
	// drop a setting such as masquerade.
	var config Config
	if err := yaml.UnmarshalWithOptions(data, &config, yaml.DisallowUnknownField()); err != nil {
		return Config{}, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := Validate(config); err != nil {
		return Config{}, err
	}

	return config, nil
}

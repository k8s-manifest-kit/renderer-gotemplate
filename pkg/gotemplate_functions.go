package gotemplate

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// ToYAML marshals a value to YAML and trims the trailing newline for cleaner
// embedding inside larger template fragments.
func ToYAML(value any) (string, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value to YAML: %w", err)
	}

	return strings.TrimSuffix(string(data), "\n"), nil
}

// Indent prefixes each line with the requested number of spaces, matching the
// behavior users commonly expect from Sprig-style template helpers.
func Indent(count int, value string) string {
	padding := strings.Repeat(" ", count)

	return padding + strings.ReplaceAll(value, "\n", "\n"+padding)
}

// Nindent prepends a newline before applying Indent, which makes block-style
// YAML template composition easier.
func Nindent(count int, value string) string {
	return "\n" + Indent(count, value)
}

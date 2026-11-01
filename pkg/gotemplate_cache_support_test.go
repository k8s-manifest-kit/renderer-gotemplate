package gotemplate_test

import (
	"fmt"
	"testing/fstest"

	jqmatcher "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// isolatedSourceTemplate renders a ConfigMap whose name and data identify the
// source filesystem the template was parsed from.
func isolatedSourceTemplate(sourceName string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-{{ .Env }}-config
data:
  source: %s
`, sourceName, sourceName)
}

// isolatedSourceFS returns a filesystem with a single template for the named source.
func isolatedSourceFS(directory string, sourceName string) fstest.MapFS {
	return fstest.MapFS{
		directory + "/config.yaml.tpl": &fstest.MapFile{
			Data: []byte(isolatedSourceTemplate(sourceName)),
		},
	}
}

// configMapSources maps each rendered ConfigMap name to its data.source value.
func configMapSources(objects []unstructured.Unstructured) map[string]string {
	sources := make(map[string]string, len(objects))

	extractSource := jqmatcher.Extract(`.data.source`)

	for _, obj := range objects {
		value, _ := extractSource(obj.Object)
		source, _ := value.(string)
		sources[obj.GetName()] = source
	}

	return sources
}

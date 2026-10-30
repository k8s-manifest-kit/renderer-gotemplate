package gotemplate_test

import (
	"fmt"
	"testing/fstest"

	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"

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

	for _, obj := range objects {
		source, _, _ := unstructured.NestedString(obj.Object, "data", "source")
		sources[obj.GetName()] = source
	}

	return sources
}

// renderOrigins maps each rendered object name to its render-origin annotation.
func renderOrigins(objects []unstructured.Unstructured) map[string]string {
	origins := make(map[string]string, len(objects))

	for _, obj := range objects {
		origin, _, _ := unstructured.NestedString(
			obj.Object,
			"metadata", "annotations", pkgtypes.AnnotationRenderOrigin,
		)
		origins[obj.GetName()] = origin
	}

	return origins
}

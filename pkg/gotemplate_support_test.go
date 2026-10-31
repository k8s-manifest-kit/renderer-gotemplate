package gotemplate_test

import (
	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const podTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Repo }}-pod
  labels:
    app: {{ .Repo }}
    component: {{ .Component }}
spec:
  containers:
  - name: nginx
    image: nginx:latest
`

const configMapTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Repo }}-config
  labels:
    app: {{ .Repo }}
    component: {{ .Component }}
data:
  config.yaml: |
    port: {{ .Port }}
`

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

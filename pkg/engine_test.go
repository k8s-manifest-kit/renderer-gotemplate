package gotemplate_test

import (
	"os"
	"testing"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

func TestNewEngine(t *testing.T) {

	t.Run("should create engine with GoTemplate renderer", func(t *testing.T) {
		g := NewWithT(t)
		e, err := gotemplate.NewEngine(gotemplate.Source{
			FS:   os.DirFS("."),
			Path: "*.go",
		})

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(e).ShouldNot(BeNil())
	})

	t.Run("should return error for invalid source", func(t *testing.T) {
		g := NewWithT(t)
		e, err := gotemplate.NewEngine(gotemplate.Source{
			// Missing FS and Path
		})

		g.Expect(err).Should(HaveOccurred())
		g.Expect(e).Should(BeNil())
	})
}

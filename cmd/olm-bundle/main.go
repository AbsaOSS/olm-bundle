package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/upbound/olm-bundle/internal/csv"
	"github.com/upbound/olm-bundle/internal/manifests"
	"github.com/upbound/olm-bundle/internal/writer"
)

type olmBundleCLI struct {
	ChartFilePath     string `help:"Path to Helm Chart.yaml file to produce metadata." type:"path" required:""`
	OutputDir         string `help:"Output directory to save the OLM bundle files." type:"path" required:""`
	ExtraResourcesDir string `help:"Extra resources you would like to add to the OLM bundle." type:"path"`
	Version           string `help:"Version of the generated bundle. If not provided, value from Chart.yaml will be used"`
}

func main() {
	cli := &olmBundleCLI{}
	ctx := kong.Parse(cli)
	var extraFiles []string
	if cli.ExtraResourcesDir != "" {
		err := filepath.Walk(cli.ExtraResourcesDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			extraFiles = append(extraFiles, path)
			return nil
		})
		ctx.FatalIfErrorf(err, "cannot walk the extra resources directory")
	}
	p := manifests.NewParser(extraFiles, os.Stdin)
	resources, err := p.Parse()
	ctx.FatalIfErrorf(err, "cannot parse resources")

	result, err := csv.NewClusterServiceVersion(cli.OutputDir)
	ctx.FatalIfErrorf(err, "cannot initialize a new ClusterServiceVersion")
	hm := &manifests.HelmMetadata{
		ChartFilePath: cli.ChartFilePath,
		Version:       cli.Version,
	}
	ctx.FatalIfErrorf(hm.Embed(context.TODO(), result), "cannot embed metadata from Helm Chart.yaml file")

	e := csv.NewEmbedder()
	remaining, err := e.Embed(resources, result)
	ctx.FatalIfErrorf(err, "cannot embed resources into ClusterServiceVersion file")

	ann, err := csv.NewAnnotations(cli.OutputDir)
	ctx.FatalIfErrorf(err, "cannot create a new annotations object")
	if result.GetAnnotations() == nil {
		result.SetAnnotations(map[string]string{})
	}
	for k, v := range ann {
		result.GetAnnotations()[k] = v
	}

	ctx.FatalIfErrorf(csv.Validate(remaining), "cannot validate")
	out := make([]client.Object, len(remaining)+1)
	for i, u := range remaining {
		out[i] = u
	}
	out[len(out)-1] = result
	b := &writer.Bundle{
		PackageDir: cli.OutputDir,
		Manifests:  out,
		Metadata: writer.Metadata{
			Annotations: ann,
		},
	}
	dir, err := b.Write()
	ctx.FatalIfErrorf(err, "cannot write bundle")

	fmt.Printf("✨ You can find your OLM bundle in %s\n🚀 Have fun!\n", dir)
}

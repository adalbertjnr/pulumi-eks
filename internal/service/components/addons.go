package components

import "pulumi-eks/internal/types"

type Extensions struct{}

func NewExtensions(components types.HelmChartsComponentes) *Extensions {
	return &Extensions{}
}

func (e *Extensions) Run(d *types.InterServicesDependencies) error {
	return nil
}

package service

import "pulumi-eks/internal/types"

type Extensions struct{}

func NewExtensions(components types.HelmChartsComponentes) *Extensions {
	return &Extensions{}
}

func (e *Extensions) Run() error {
	return nil
}

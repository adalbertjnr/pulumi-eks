package service

import (
	"pulumi-eks/internal/types"

	helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Extensions struct {
	ctx            *pulumi.Context
	helmComponents types.HelmChartsComponentes
}

func NewExtensions(ctx *pulumi.Context, components types.HelmChartsComponentes) *Extensions {
	return &Extensions{
		ctx:            ctx,
		helmComponents: components,
	}
}

func (e *Extensions) Run(dependency *types.InterServicesDependencies) error {
	return e.applyHelmCharts(dependency)
}

func (e *Extensions) applyHelmCharts(dependency *types.InterServicesDependencies) error {
	var nodeGroupResourceList = make([]pulumi.Resource, len(dependency.NodeGroupsOutput.NodeGroups))

	for n, nodeGroupOutput := range dependency.NodeGroupsOutput.NodeGroups {
		nodeGroupResourceList[n] = nodeGroupOutput
	}

	dependsOnResources := append(
		[]pulumi.Resource{dependency.ClusterOutput.EKSCluster},
		nodeGroupResourceList...,
	)

	for _, component := range e.helmComponents.Components {
		_, err := helmv4.NewChart(e.ctx, component.Name, &helmv4.ChartArgs{
			Name:      pulumi.String(component.Name),
			Chart:     pulumi.String(component.Name),
			Namespace: pulumi.String(component.Namespace),
			SkipCrds:  pulumi.BoolPtr(component.SkipCirds),
			Version:   pulumi.StringPtr(*component.Version),
			RepositoryOpts: helmv4.RepositoryOptsArgs{
				Repo: pulumi.String(component.Repository),
			},
			Values: pulumi.ToMap(component.SetValues),
		}, pulumi.DependsOn(dependsOnResources))

		if err != nil {
			return err
		}
	}

	return nil
}

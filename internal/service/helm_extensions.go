package service

import (
	"pulumi-eks/internal/service/shared"
	"pulumi-eks/internal/types"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
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
	dependsOn := shared.RetrieveDependsOnList(dependency)

	provider, err := kubernetes.NewProvider(e.ctx, "kubernetes-provider-helm", &kubernetes.ProviderArgs{
		Kubeconfig: dependency.ClusterOutput.KubeConfig,
	})

	if err != nil {
		return err
	}

	for _, component := range e.helmComponents.Components {
		_, err := helmv4.NewChart(e.ctx, component.Name, &helmv4.ChartArgs{
			Name:      pulumi.String(component.Name),
			Chart:     pulumi.String(component.Name),
			Namespace: pulumi.String(component.Namespace),
			SkipCrds:  pulumi.BoolPtr(component.SkipCirds),
			Version:   pulumi.String(*component.Version),
			RepositoryOpts: helmv4.RepositoryOptsArgs{
				Repo: pulumi.String(component.Repository),
			},
			Values: pulumi.ToMap(component.SetValues),
		}, pulumi.DependsOn(dependsOn), pulumi.Provider(provider))

		if err != nil {
			return err
		}
	}

	return nil
}

package main

import (
	"pulumi-eks/internal/command"
	asg "pulumi-eks/internal/service/autoscaling"
	"pulumi-eks/internal/service/components"
	"pulumi-eks/internal/service/eks"
	"pulumi-eks/internal/service/networking"
	"pulumi-eks/internal/types"
	cfgreader "pulumi-eks/pkg/read"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		c := &types.Config{}

		config := cfgreader.NewAppConfigReader()
		err := config.ReadFrom("../config.yaml").Decode(c)
		if err != nil {
			return err
		}

		resourceController := command.New()

		networkingService := networking.NewNetworking(
			ctx,
			c.Spec.Networking,
		)

		autoscalingService := asg.NewAutoscalingGroup(
			ctx,
			c.Spec.Cluster,
			c.Spec.NodeGroups,
		)

		clusterService := eks.NewClusterEKS(
			ctx,
			c.Spec.Networking,
			c.Spec.Cluster,
			c.Spec.NodeGroups,
		)

		extensionsService := components.NewExtensions(
			c.Spec.HelmChartsComponentes,
		)

		resourceController.AddCommand(
			networkingService,
			autoscalingService,
			clusterService,
			extensionsService,
		)

		interServicesDependsOn := &types.InterServicesDependencies{}

		return resourceController.RunCommands(
			interServicesDependsOn,
		)
	})
}

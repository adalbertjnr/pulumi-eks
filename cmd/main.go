package main

import (
	"pulumi-eks/internal/command"
	"pulumi-eks/internal/service"
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

		networkingService := service.NewNetworking(
			ctx,
			c.Spec.Networking,
		)

		autoscalingService := service.NewAutoscalingGroup(
			ctx,
			c.Spec.Cluster,
			c.Spec.NodeGroups,
		)

		clusterService := service.NewClusterEKS(
			ctx,
			c.Spec.Networking,
			c.Spec.Cluster,
			c.Spec.NodeGroups,
		)

		nodeGroupService := service.NewNodeGroup(
			ctx,
			c.Spec.Networking,
			c.Spec.Cluster,
			c.Spec.NodeGroups,
		)

		extensionsService := service.NewExtensions(
			c.Spec.HelmChartsComponentes,
		)

		resourceController.AddCommand(
			networkingService,
			clusterService,
			autoscalingService,
			nodeGroupService,
			extensionsService,
		)

		interServicesDependsOn := &types.InterServicesDependencies{}

		return resourceController.RunCommands(
			interServicesDependsOn,
		)
	})
}

package shared

import (
	"pulumi-eks/internal/types"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func RetrieveDependsOnList(dependency *types.InterServicesDependencies) []pulumi.Resource {
	var nodeGroupResourceList = make([]pulumi.Resource, len(dependency.NodeGroupsOutput.NodeGroups))

	for n, nodeGroupOutput := range dependency.NodeGroupsOutput.NodeGroups {
		nodeGroupResourceList[n] = nodeGroupOutput
	}

	dependsOn := append(
		[]pulumi.Resource{dependency.ClusterOutput.EKSCluster},
		nodeGroupResourceList...,
	)

	return dependsOn
}

package service

import (
	"fmt"
	"pulumi-eks/internal/types"
	"pulumi-eks/pkg/generic"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type NodeGroup struct {
	ctx        *pulumi.Context
	networking types.Networking
	cluster    types.Cluster
	nodes      []types.NodeGroups
}

func NewNodeGroup(ctx *pulumi.Context, networking types.Networking, cluster types.Cluster, nodes []types.NodeGroups) *NodeGroup {
	return &NodeGroup{
		ctx:        ctx,
		networking: networking,
		cluster:    cluster,
		nodes:      nodes,
	}
}

func (c *NodeGroup) Run(networkingDependency *types.InterServicesDependencies) error {
	return nil
}

func (c *NodeGroup) createNodeGroup(servicesDependencies *types.InterServicesDependencies) error {
	privateSubnetList, found := servicesDependencies.Subnets[types.PRIVATE_SUBNET]
	if !found {
		return fmt.Errorf("private subnets were not found in the subnets map")
	}

	pulumiIDOutputList := generic.ToStringOutputList(privateSubnetList, func(subnet *ec2.Subnet) pulumi.StringOutput {
		return subnet.ID().ToStringOutput()
	})

	for nodeName, nodeGroupConfig := range servicesDependencies.LaunchTemplateOutputList {
		_, err := eks.NewNodeGroup(c.ctx, nodeName, &eks.NodeGroupArgs{
			SubnetIds:     pulumi.ToStringArrayOutput(pulumiIDOutputList),
			NodeGroupName: pulumi.String(strings.ToUpper(nodeName)),
			Tags:          pulumi.ToStringMap(),
			LaunchTemplate: eks.NodeGroupLaunchTemplateArgs{
				Id: nodeGroupConfig.Lt.ID(),
			},
		})

		if err != nil {
			return err
		}
	}

	return nil
}

package service

import (
	"encoding/json"
	"fmt"
	"pulumi-eks/internal/types"
	"pulumi-eks/pkg/generic"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type NodeGroup struct {
	ctx        *pulumi.Context
	networking types.Networking
	cluster    types.Cluster
	nodes      []types.NodeGroups

	dependencies
}

type dependencies struct {
	att      []*iam.RolePolicyAttachment
	nodeRole *iam.Role
}

func NewNodeGroup(ctx *pulumi.Context, networking types.Networking, cluster types.Cluster, nodes []types.NodeGroups) *NodeGroup {
	return &NodeGroup{
		ctx:        ctx,
		networking: networking,
		cluster:    cluster,
		nodes:      nodes,
	}
}

func (c *NodeGroup) Run(dependency *types.InterServicesDependencies) error {
	steps := []func() error{
		func() error { return c.createNodeRole() },
		func() error { return c.createNodeGroup(dependency) },
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

func (c *NodeGroup) createNodeGroup(dependency *types.InterServicesDependencies) error {
	privateSubnetList, found := dependency.Subnets[types.PRIVATE_SUBNET]
	if !found {
		return fmt.Errorf("private subnets were not found in the subnets map")
	}

	pulumiIDOutputList := generic.ToStringOutputList(privateSubnetList, func(subnet *ec2.Subnet) pulumi.StringOutput {
		return subnet.ID().ToStringOutput()
	})

	var policyAttachmentDependsOn = make([]pulumi.Resource, len(c.att))
	for i := range c.att {
		policyAttachmentDependsOn[i] = c.att[i]
	}

	var nodeGroupOutputList types.NodeGroupsOutput

	for nodeName, nodeGroupConfig := range dependency.LaunchTemplateOutputList {
		nodeGroupOutput, err := eks.NewNodeGroup(c.ctx, nodeName, &eks.NodeGroupArgs{
			ClusterName:   dependency.ClusterOutput.EKSCluster.Name,
			NodeRoleArn:   c.dependencies.nodeRole.Arn,
			SubnetIds:     pulumi.ToStringArrayOutput(pulumiIDOutputList),
			NodeGroupName: pulumi.String(strings.ToUpper(nodeName)),
			Tags:          pulumi.ToStringMap(nodeGroupConfig.Node.NodeLabels),
			LaunchTemplate: eks.NodeGroupLaunchTemplateArgs{
				Id:      nodeGroupConfig.Lt.ID(),
				Version: pulumi.String("$Latest"),
			},
			ScalingConfig: eks.NodeGroupScalingConfigArgs{
				MinSize:     pulumi.Int(nodeGroupConfig.Node.ScalingConfig.MinSize),
				MaxSize:     pulumi.Int(nodeGroupConfig.Node.ScalingConfig.MaxSize),
				DesiredSize: pulumi.Int(nodeGroupConfig.Node.ScalingConfig.DesiredSize),
			},
		}, pulumi.DependsOn(policyAttachmentDependsOn))

		if err != nil {
			return err
		}

		nodeGroupOutputList.NodeGroups = append(nodeGroupOutputList.NodeGroups, nodeGroupOutput)
	}

	dependency.NodeGroupsOutput = nodeGroupOutputList

	return nil
}

func (c *NodeGroup) createNodeRole() error {
	nodePolicyJSON, err := json.Marshal(map[string]interface{}{
		"Statement": []map[string]interface{}{
			{
				"Action": "sts:AssumeRole",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"Service": "ec2.amazonaws.com",
				},
			},
		},
		"Version": "2012-10-17",
	})

	if err != nil {
		return err
	}

	nodePolicy := string(nodePolicyJSON)

	nodeRoleName := fmt.Sprintf("%s-noderole", c.cluster.Name)
	nodeRole, err := iam.NewRole(c.ctx, nodeRoleName, &iam.RoleArgs{
		Name:             pulumi.String(nodeRoleName),
		AssumeRolePolicy: pulumi.String(nodePolicy),
	})

	nodePolicyMap := map[string]string{
		"AmazonEKSWorkerNodePolicy":          "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"AmazonEKS_CNI_Policy":               "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		"AmazonEC2ContainerRegistryReadOnly": "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	}

	var policyAttachmentList []*iam.RolePolicyAttachment

	for policyUniqueName, policyArn := range nodePolicyMap {
		policyAttachmentOutput, err := iam.NewRolePolicyAttachment(c.ctx, policyUniqueName, &iam.RolePolicyAttachmentArgs{
			Role:      nodeRole,
			PolicyArn: pulumi.String(policyArn),
		})
		if err != nil {
			return err
		}

		policyAttachmentList = append(policyAttachmentList, policyAttachmentOutput)
	}

	c.dependencies.nodeRole = nodeRole
	c.dependencies.att = policyAttachmentList

	return nil
}

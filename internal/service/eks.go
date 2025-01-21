package service

import (
	"encoding/json"
	"fmt"
	"pulumi-eks/internal/types"
	"pulumi-eks/pkg/generic"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ClusterEKS struct {
	ctx        *pulumi.Context
	networking types.Networking
	cluster    types.Cluster
	nodes      []types.NodeGroups

	dependencies clusterDependsOn
}

type clusterDependsOn struct {
	clusterRoleAttachment *iam.RolePolicyAttachment
	clusterRole           *iam.Role
}

func NewClusterEKS(ctx *pulumi.Context, networking types.Networking, cluster types.Cluster, nodes []types.NodeGroups) *ClusterEKS {
	return &ClusterEKS{
		ctx:        ctx,
		networking: networking,
		cluster:    cluster,
		nodes:      nodes,
	}
}

func (c *ClusterEKS) Run(d *types.InterServicesDependencies) error {
	steps := []func() error{
		func() error { return c.createEKSRole() },
		func() error { return c.createEKSCluster(d) },
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

func (c *ClusterEKS) createEKSCluster(d *types.InterServicesDependencies) error {
	pulumiIDOutputList := generic.ToStringOutputList(
		d.Subnets, func(subnet *ec2.Subnet) pulumi.StringOutput {
			return pulumi.StringOutput(subnet.ID())
		})

	eksClusterOutput, err := eks.NewCluster(c.ctx, c.cluster.Name, &eks.ClusterArgs{
		Name:    pulumi.String(c.cluster.Name),
		Version: pulumi.String(c.cluster.KubernetesVersion),
		AccessConfig: &eks.ClusterAccessConfigArgs{
			AuthenticationMode: pulumi.String("API"),
		},
		RoleArn: c.dependencies.clusterRole.Arn,
		VpcConfig: &eks.ClusterVpcConfigArgs{
			SubnetIds: pulumi.ToStringArrayOutput(pulumiIDOutputList),
		},
	}, pulumi.DependsOn([]pulumi.Resource{
		c.dependencies.clusterRoleAttachment,
	}))

	if err != nil {
		return err
	}

	_ = eksClusterOutput

	return nil
}

func (c *ClusterEKS) createEKSRole() error {
	clusterPolicyJSON, err := json.Marshal(map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Action": []string{
					"sts:AssumeRole",
					"sts:TagSession",
				},
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"Service": "eks.amazonaws.com",
				},
			},
		},
	})

	if err != nil {
		return err
	}

	clusterPolicy := string(clusterPolicyJSON)

	clusterRoleName := fmt.Sprintf("%s-clusterrole", c.cluster.Name)
	clusterRole, err := iam.NewRole(c.ctx, clusterRoleName, &iam.RoleArgs{
		Name:             pulumi.String(clusterRoleName),
		AssumeRolePolicy: pulumi.String(clusterPolicy),
	})
	if err != nil {
		return err
	}

	attachmentRoleName := clusterRoleName + "-attachment"
	roleAttachment, err := iam.NewRolePolicyAttachment(c.ctx, attachmentRoleName, &iam.RolePolicyAttachmentArgs{
		Role:      clusterRole,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"),
	})

	c.dependencies.clusterRoleAttachment = roleAttachment
	c.dependencies.clusterRole = clusterRole

	return err
}

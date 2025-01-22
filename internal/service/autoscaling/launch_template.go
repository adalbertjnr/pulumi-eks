package asg

import (
	"fmt"
	"pulumi-eks/internal/types"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type AutoscalingGroup struct {
	ctx *pulumi.Context

	lt *ec2.LaunchTemplate
}

func NewAutoscalingGroup(ctx *pulumi.Context, c types.Cluster, n []types.NodeGroups) *AutoscalingGroup {
	return &AutoscalingGroup{ctx: ctx}
}

func (ag *AutoscalingGroup) Run(interServicesDependencies *types.InterServicesDependencies) error {
	return nil
}
func (ag *AutoscalingGroup) createAutoscalingGroup(interServicesDependencies *types.InterServicesDependencies) error {
	asgUniqueName := fmt.Sprintf("%s-asg", "foo")
	autoScalingGroupOutput, err := autoscaling.NewGroup(ag.ctx, asgUniqueName, &autoscaling.GroupArgs{
		MaxSize:         pulumi.Int(3),
		MinSize:         pulumi.Int(1),
		DesiredCapacity: pulumi.Int(2),
		LaunchTemplate: autoscaling.GroupLaunchTemplateArgs{
			Id: ag.lt.ID(),
		},
	}, pulumi.DependsOn([]pulumi.Resource{ag.lt}))

	interServicesDependencies.AutoscalingGroup = autoScalingGroupOutput

	return err
}

func (ag *AutoscalingGroup) launchTemplate(interServicesDependencies *types.InterServicesDependencies) error {
	ltUniqueName := fmt.Sprintf("%s-lt", "foo")

	launchTemplateOutput, err := ec2.NewLaunchTemplate(ag.ctx, ltUniqueName, &ec2.LaunchTemplateArgs{
		Name:                 pulumi.String(ltUniqueName),
		UpdateDefaultVersion: pulumi.Bool(true),
		ImageId:              pulumi.String("ami-03413b57906e5c8b2"),
		InstanceType:         pulumi.String("t3.medium"),
		UserData:             pulumi.String(LAUNCH_TEMPLATE_USERDATA),

		BlockDeviceMappings: ec2.LaunchTemplateBlockDeviceMappingArray{
			ec2.LaunchTemplateBlockDeviceMappingArgs{
				DeviceName: pulumi.String("/dev/xvda"),
				Ebs: ec2.LaunchTemplateBlockDeviceMappingEbsArgs{
					VolumeSize:          pulumi.Int(30),
					VolumeType:          pulumi.String("gp3"),
					Encrypted:           pulumi.String("true"),
					DeleteOnTermination: pulumi.String("true"),
				},
			},
		},

		MetadataOptions: ec2.LaunchTemplateMetadataOptionsArgs{
			HttpPutResponseHopLimit: pulumi.Int(2),
			HttpEndpoint:            pulumi.String("enabled"),
			HttpTokens:              pulumi.String("required"),
		},

		TagSpecifications: ec2.LaunchTemplateTagSpecificationArray{
			ec2.LaunchTemplateTagSpecificationArgs{
				ResourceType: pulumi.String("instance"),
				Tags:         pulumi.ToStringMap(map[string]string{"": ""}),
			},
			ec2.LaunchTemplateTagSpecificationArgs{
				ResourceType: pulumi.String("volume"),
				Tags:         pulumi.ToStringMap(map[string]string{"": ""}),
			},
		},
	})

	ag.lt = launchTemplateOutput

	return err
}

const LAUNCH_TEMPLATE_USERDATA = `
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="==MYBOUNDARY=="

--==MYBOUNDARY==
Content-Type: text/x-shellscript; charset="us-ascii"

#!/bin/bash
set -ex

exec > >(tee /var/log/user-data.log|logger -t user-data -s 2>/dev/console) 2>&1

# Install Docker
amazon-linux-extras install docker
systemctl enable docker
systemctl start docker

yum install -y amazon-ssm-agent htop
systemctl enable amazon-ssm-agent && systemctl start amazon-ssm-agent

/etc/eks/bootstrap.sh ${CLUSTER_NAME} --b64-cluster-ca ${B64_CLUSTER_CA} --apiserver-endpoint ${API_SERVER_URL}

--==MYBOUNDARY==--\
`

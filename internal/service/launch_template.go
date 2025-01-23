package service

import (
	"bytes"
	"fmt"
	"html/template"
	"pulumi-eks/internal/types"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type AutoscalingGroup struct {
	ctx *pulumi.Context

	cluster types.Cluster
	nodes   []types.NodeGroups

	lt *ec2.LaunchTemplate
}

func NewAutoscalingGroup(ctx *pulumi.Context, c types.Cluster, n []types.NodeGroups) *AutoscalingGroup {
	return &AutoscalingGroup{
		ctx:     ctx,
		cluster: c,
		nodes:   n,
	}
}

func (ag *AutoscalingGroup) Run(dependency *types.InterServicesDependencies) error {
	return ag.launchTemplate(dependency)
}

// func (ag *AutoscalingGroup) createAutoscalingGroup(interServicesDependencies *types.InterServicesDependencies) error {
// 	asgUniqueName := fmt.Sprintf("%s-asg", "foo")
// 	autoScalingGroupOutput, err := autoscaling.NewGroup(ag.ctx, asgUniqueName, &autoscaling.GroupArgs{
// 		MaxSize:         pulumi.Int(3),
// 		MinSize:         pulumi.Int(1),
// 		DesiredCapacity: pulumi.Int(2),
// 		LaunchTemplate: autoscaling.GroupLaunchTemplateArgs{
// 			Id: ag.lt.ID(),
// 		},
// 	}, pulumi.DependsOn([]pulumi.Resource{ag.lt}))

// 	interServicesDependencies.AutoscalingGroup = autoScalingGroupOutput

// 	return err
// }

func (ag *AutoscalingGroup) launchTemplate(dependency *types.InterServicesDependencies) error {

	const INSTANCE = "instance"
	const VOLUME = "volume"

	var launchTemplateOutputMap = make(map[string]types.NodeGroupMetadata, len(ag.nodes))

	clusterOutput := dependency.ClusterOutput

	clusterUserData := pulumi.All(
		clusterOutput.EKSCluster.Name,
		clusterOutput.EKSCluster.CertificateAuthority.Data(),
		clusterOutput.EKSCluster.Endpoint,
	).
		ApplyT(func(args []interface{}) (string, error) {
			clusterName := args[0].(string)
			ca := args[1].(*string)
			endpoint := args[2].(string)

			return buildLauncTemplateUserData(clusterName, *ca, endpoint)
		}).(pulumi.StringOutput)

	ag.ctx.Export("test", clusterUserData)

	for n, node := range ag.nodes {
		launchTemplateUniqueName := fmt.Sprintf("%s-lt-%d", node.Name, n)

		launchTemplateOutput, err := ec2.NewLaunchTemplate(ag.ctx, launchTemplateUniqueName, &ec2.LaunchTemplateArgs{
			Name:                 pulumi.String(launchTemplateUniqueName),
			UpdateDefaultVersion: pulumi.Bool(true),
			ImageId:              pulumi.String(node.ImageId),
			InstanceType:         pulumi.String(node.InstanceType),
			UserData:             clusterUserData.ToStringPtrOutput(),

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
					ResourceType: pulumi.String(INSTANCE),
					Tags: pulumi.ToStringMap(map[string]string{
						"Name": fmt.Sprintf("%s-%s-%s", ag.cluster.Name, node.Name, INSTANCE),
					}),
				},

				ec2.LaunchTemplateTagSpecificationArgs{
					ResourceType: pulumi.String(VOLUME),
					Tags: pulumi.ToStringMap(map[string]string{
						"Name": fmt.Sprintf("%s-%s-%s", ag.cluster.Name, node.Name, VOLUME)},
					),
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{dependency.ClusterOutput.EKSCluster}))

		if err != nil {
			return err
		}

		if _, exists := launchTemplateOutputMap[node.Name]; !exists {
			launchTemplateOutputMap[node.Name] = types.NodeGroupMetadata{
				Node: node,
				Lt:   launchTemplateOutput,
			}
		}
	}
	dependency.LaunchTemplateOutputList = launchTemplateOutputMap
	// ag.lt = launchTemplateOutput

	return nil
}

func buildLauncTemplateUserData(clusterName, clusterCA, clusterAPIServerURL string) (string, error) {
	const LAUNCH_TEMPLATE_USERDATA = `
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="==MYBOUNDARY=="

--==MYBOUNDARY==
Content-Type: text/x-shellscript; charset="us-ascii"

#!/bin/bash
set -ex

exec > >(tee /var/log/user-data.log|logger -t user-data -s 2>/dev/console) 2>&1

amazon-linux-extras install docker
systemctl enable docker
systemctl start docker

yum install -y amazon-ssm-agent htop
systemctl enable amazon-ssm-agent && systemctl start amazon-ssm-agent

/etc/eks/bootstrap.sh {{ .ClusterName }} --b64-cluster-ca {{ .ClusterCA }} --apiserver-endpoint {{ .ApiServerUrl }}

--==MYBOUNDARY==--\
`

	ltData := struct {
		ClusterName  string
		ClusterCA    string
		ApiServerUrl string
	}{
		ClusterName:  clusterName,
		ClusterCA:    clusterCA,
		ApiServerUrl: clusterAPIServerURL,
	}

	tmpl, err := template.New("userData").Parse(LAUNCH_TEMPLATE_USERDATA)
	if err != nil {
		return "", err
	}

	var r bytes.Buffer
	if err := tmpl.Execute(&r, ltData); err != nil {
		return "", err
	}

	return r.String(), nil
}

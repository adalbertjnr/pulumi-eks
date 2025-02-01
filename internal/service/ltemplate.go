package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"pulumi-eks/internal/types"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type LaunchTemplate struct {
	ctx *pulumi.Context

	cluster types.Cluster
	nodes   []types.NodeGroups

	lt *ec2.LaunchTemplate
}

func NewLaunchTemplate(ctx *pulumi.Context, cluster types.Cluster, nodes []types.NodeGroups) *LaunchTemplate {
	return &LaunchTemplate{
		ctx:     ctx,
		cluster: cluster,
		nodes:   nodes,
	}
}

func (ag *LaunchTemplate) Run(dependency *types.InterServicesDependencies) error {
	return ag.launchTemplate(dependency)
}

func (ag *LaunchTemplate) launchTemplate(dependency *types.InterServicesDependencies) error {

	var launchTemplateOutputMap = make(map[string]types.NodeGroupMetadata, len(ag.nodes))

	for n, node := range ag.nodes {
		launchTemplateUniqueName := fmt.Sprintf("%s-lt-%d", node.Name, n)

		clusterUserData := createLtUserData(
			dependency.ClusterOutput,
			node.Name,
		)

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
					ResourceType: pulumi.String("instance"),
					Tags: pulumi.ToStringMap(map[string]string{
						"Name": fmt.Sprintf("%s-%s-%s", ag.cluster.Name, node.Name, "instance"),
					}),
				},

				ec2.LaunchTemplateTagSpecificationArgs{
					ResourceType: pulumi.String("volume"),
					Tags: pulumi.ToStringMap(map[string]string{
						"Name": fmt.Sprintf("%s-%s-%s", ag.cluster.Name, node.Name, "volume")},
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

	return nil
}

func createLtUserData(clusterOutput types.ClusterOutput, nodeGroupName string) pulumi.StringOutput {
	return pulumi.All(
		clusterOutput.EKSCluster.Name,
		clusterOutput.EKSCluster.CertificateAuthority.Data(),
		clusterOutput.EKSCluster.Endpoint,
		clusterOutput.EKSCluster.KubernetesNetworkConfig.ServiceIpv4Cidr(),
	).
		ApplyT(func(args []interface{}) (string, error) {
			clusterName := args[0].(string)
			ca := args[1].(*string)
			endpoint := args[2].(string)
			clusterCidr := args[3].(*string)

			return buildLauncTemplateUserData(clusterName, *ca, endpoint, *clusterCidr, strings.ToUpper(nodeGroupName))
		}).(pulumi.StringOutput)
}

func buildLauncTemplateUserData(clusterName, clusterCA, clusterAPIServerURL, clusterCIDR string, nodeGroupName string) (string, error) {
	const LAUNCH_TEMPLATE_USERDATA = `MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="//"

--//
Content-Type: application/node.eks.aws

---
apiVersion: node.eks.aws/v1alpha1
kind: NodeConfig
spec:
  cluster:
    apiServerEndpoint: {{ .ApiServerUrl }}
    certificateAuthority: {{ .ClusterCA }}
    cidr: {{ .ClusterCIDR }}
    name: {{ .ClusterName }}
  kubelet:
    flags:
    - "--node-labels=eks.amazonaws.com/nodegroup={{ .NodeGroupName }}"

--//
Content-Type: text/x-shellscript; charset="us-ascii"

#!/bin/bash
set -o xtrace

yum install amazon-ssm-agent -y
systemctl enable amazon-ssm-agent && systemctl start amazon-ssm-agent
  
--//--`

	ltData := struct {
		ClusterName   string
		ClusterCA     string
		ApiServerUrl  string
		ClusterCIDR   string
		NodeGroupName string
	}{
		ClusterName:   clusterName,
		ClusterCA:     clusterCA,
		ApiServerUrl:  clusterAPIServerURL,
		ClusterCIDR:   clusterCIDR,
		NodeGroupName: nodeGroupName,
	}

	tmpl, err := template.New("userData").Parse(LAUNCH_TEMPLATE_USERDATA)
	if err != nil {
		return "", err
	}

	var r bytes.Buffer
	if err := tmpl.Execute(&r, ltData); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(r.Bytes()), nil
}

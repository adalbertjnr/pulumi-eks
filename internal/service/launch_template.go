package service

import (
	"bytes"
	"encoding/base64"
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

	clusterOutput.EKSCluster.KubernetesNetworkConfig.ServiceIpv4Cidr()

	clusterUserData := pulumi.All(
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

			return buildLauncTemplateUserData(clusterName, *ca, endpoint, *clusterCidr)
		}).(pulumi.StringOutput)

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

func buildLauncTemplateUserData(clusterName, clusterCA, clusterAPIServerURL, clusterCIDR string) (string, error) {
	const LAUNCH_TEMPLATE_USERDATA = `
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="BOUNDARY"

--BOUNDARY
Content-Type: application/node.eks.aws

---
apiVersion: node.eks.aws/v1alpha1
kind: NodeConfig
spec:
  cluster:
    name: {{ .ClusterName }}
    apiServerEndpoint: {{ .ApiServerUrl }}
    certificateAuthority: {{ .ClusterCA }}
    cidr: {{ .ClusterCIDR }}

--BOUNDARY--
	`

	ltData := struct {
		ClusterName  string
		ClusterCA    string
		ApiServerUrl string
		ClusterCIDR  string
	}{
		ClusterName:  clusterName,
		ClusterCA:    clusterCA,
		ApiServerUrl: clusterAPIServerURL,
		ClusterCIDR:  clusterCIDR,
	}

	tmpl, err := template.New("userData").Parse(LAUNCH_TEMPLATE_USERDATA)
	if err != nil {
		return "", err
	}

	var r bytes.Buffer
	if err := tmpl.Execute(&r, ltData); err != nil {
		return "", err
	}

	userDataBase64 := base64.StdEncoding.EncodeToString(r.Bytes())

	return userDataBase64, nil
}

package types

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
)

type SubnetType int

const (
	PRIVATE_SUBNET SubnetType = iota
	PUBLIC_SUBNET
)

const PUBLIC_CIDR = "0.0.0.0/0"

type InterServicesDependencies struct {
	Subnets map[SubnetType][]*ec2.Subnet

	AutoscalingGroup         *autoscaling.Group
	LaunchTemplateOutputList map[string]NodeGroupMetadata

	ClusterOutput ClusterOutput
}

type NodeGroupMetadata struct {
	Node NodeGroups
	Lt   *ec2.LaunchTemplate
}

type ClusterOutput struct {
	EKSCluster *eks.Cluster
}

type Config struct {
	Spec Spec `yaml:"spec"`
}
type Networking struct {
	Name      string `yaml:"name"`
	CidrBlock string `yaml:"cidrBlock"`
	Subnets   []Subnets
}
type Subnets struct {
	Name             string `yaml:"name"`
	CidrBlock        string `yaml:"cidrBlock"`
	PublicIpOnLaunch bool   `yaml:"publicIpOnLaunch"`
	AvailabilityZone string `yaml:"availabilityZone"`
}

type Cluster struct {
	Name              string   `yaml:"name"`
	Environment       string   `yaml:"environment"`
	Region            string   `yaml:"region"`
	KubernetesVersion string   `yaml:"kubernetesVersion"`
	VpcID             string   `yaml:"vpcId"`
	Subnets           []string `yaml:"subnets"`
	SecurityGroups    []string `yaml:"securityGroups"`
}
type ScalingConfig struct {
	MinSize     int `yaml:"minSize"`
	DesiredSize int `yaml:"desiredSize"`
	MaxSize     int `yaml:"maxSize"`
}
type NodeGroups struct {
	Name          string            `yaml:"name"`
	ScalingConfig ScalingConfig     `yaml:"scalingConfig"`
	InstanceType  string            `yaml:"instanceType"`
	NodeLabels    map[string]string `yaml:"nodeLabels"`
	ImageId       string            `yaml:"imageId"`
}
type SetValues struct {
	Foo string `yaml:"foo"`
}
type Components struct {
	Name            string    `yaml:"name"`
	Version         string    `yaml:"version"`
	Repository      string    `yaml:"repository"`
	Namespace       string    `yaml:"namespace"`
	SetValues       SetValues `yaml:"setValues"`
	CreateNamespace bool      `yaml:"createNamespace,omitempty"`
}
type HelmChartsComponentes struct {
	Components []Components `yaml:"components"`
}
type Spec struct {
	Networking            Networking            `yaml:"networking"`
	Cluster               Cluster               `yaml:"cluster"`
	NodeGroups            []NodeGroups          `yaml:"nodeGroups"`
	HelmChartsComponentes HelmChartsComponentes `yaml:"helmChartsComponentes"`
}

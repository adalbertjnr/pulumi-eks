package types

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	yamlv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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

	NodeGroupsOutput NodeGroupsOutput

	PodIdentityAgent *yamlv2.ConfigGroup
}
type NodeGroupMetadata struct {
	Node NodeGroups
	Lt   *ec2.LaunchTemplate
}

type NodeGroupsOutput struct {
	NodeGroups []*eks.NodeGroup
}

type ClusterOutput struct {
	EKSCluster *eks.Cluster
	KubeConfig pulumi.StringOutput
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
	Name             string                 `yaml:"name"`
	CidrBlock        string                 `yaml:"cidrBlock"`
	PublicIpOnLaunch bool                   `yaml:"publicIpOnLaunch"`
	AvailabilityZone string                 `yaml:"availabilityZone"`
	Tags             map[string]interface{} `yaml:"tags"`
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
type Components struct {
	Name             string                 `yaml:"name"`
	Version          *string                `yaml:"version"`
	Repository       string                 `yaml:"repository"`
	Namespace        string                 `yaml:"namespace"`
	SetValues        map[string]interface{} `yaml:"setValues"`
	CreateNamespace  bool                   `yaml:"createNamespace,omitempty"`
	SkipCirds        bool                   `yaml:"skipCirds"`
	WithOIDCProvider *WithOIDCProvider      `yaml:"withOidcProvider"`
}

type WithOIDCProvider struct {
	Create         bool           `yaml:"create"`
	ServiceAccount ServiceAccount `yaml:"serviceAccount"`
	OidcIAMRole    OidcIAMRole    `yaml:"role"`
}

type OidcIAMRole struct {
	Name                    string   `yaml:"name"`
	AwsPolicies             []string `yaml:"awsPolicies"`
	SelfManagedPoliciesPath []string `yaml:"selfManagedPoliciesPath"`
}

type ServiceAccount struct {
	Name string `yaml:"name"`
}

type HelmChartsComponentes struct {
	Components []Components `yaml:"components"`
}

type IdentityPodAgent struct {
	Deploy     bool       `yaml:"deploy"`
	Identities Identities `yaml:"identities"`
}

type Identities struct {
	Roles         []Role         `yaml:"roles"`
	Relationships []Relationship `yaml:"relationships"`
}

type Role struct {
	RoleName                string   `yaml:"roleName"`
	AwsPolicies             []string `yaml:"awsPolicies"`
	SelfManagedPoliciesPath []string `yaml:"selfManagedPoliciesPath"`
}

type Relationship struct {
	RoleName  string `yaml:"roleName"`
	Namespace string `yaml:"namespace"`
}

type Spec struct {
	Networking            Networking            `yaml:"networking"`
	Cluster               Cluster               `yaml:"cluster"`
	NodeGroups            []NodeGroups          `yaml:"nodeGroups"`
	HelmChartsComponentes HelmChartsComponentes `yaml:"helmChartsComponentes"`
	IdentityPodAgent      IdentityPodAgent      `yaml:"identityPodAgent"`
}

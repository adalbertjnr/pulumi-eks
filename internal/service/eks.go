package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"pulumi-eks/internal/types"
	"pulumi-eks/pkg/generic"
	"text/template"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/vpc"
	pulumiyaml "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/yaml"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ClusterEKS struct {
	ctx        *pulumi.Context
	networking types.Networking
	cluster    types.Cluster
	nodes      []types.NodeGroups

	clusterOutput *eks.Cluster

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

func (c *ClusterEKS) Run(dependency *types.InterServicesDependencies) error {
	steps := []func() error{
		func() error { return c.createEKSRole() },
		func() error { return c.createEKSCluster(dependency) },
		func() error { return c.modifyEKSSecurityGroup() },
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

func (c *ClusterEKS) createEKSCluster(dependency *types.InterServicesDependencies) error {
	publicSubnetList, found := dependency.Subnets[types.PUBLIC_SUBNET]
	if !found {
		return fmt.Errorf("public subnets were not found in the subnets map")
	}

	pulumiIDOutputList := generic.ToStringOutputList(
		publicSubnetList, func(subnet *ec2.Subnet) pulumi.StringOutput {
			return subnet.ID().ToStringOutput()
		})

	clusterOutput, err := eks.NewCluster(c.ctx, c.cluster.Name, &eks.ClusterArgs{
		Name:    pulumi.String(c.cluster.Name),
		Version: pulumi.String(c.cluster.KubernetesVersion),
		RoleArn: c.dependencies.clusterRole.Arn,
		VpcConfig: &eks.ClusterVpcConfigArgs{
			SubnetIds:             pulumi.ToStringArrayOutput(pulumiIDOutputList),
			EndpointPrivateAccess: pulumi.BoolPtr(true),
			EndpointPublicAccess:  pulumi.BoolPtr(true),
		},
	}, pulumi.DependsOn([]pulumi.Resource{
		c.dependencies.clusterRoleAttachment,
	}))

	if err != nil {
		return err
	}

	clusterOutput.Name.ApplyT(func(name string) error {
		return deployPodIdentityAgent(c.ctx, name, c.cluster.Region, clusterOutput)
	})

	kubeConfig := generateKubeconfig(
		clusterOutput.Endpoint,
		clusterOutput.CertificateAuthority.Data().Elem(),
		clusterOutput.Name,
	)

	clusterOutputDTO := types.ClusterOutput{
		EKSCluster: clusterOutput,
		KubeConfig: kubeConfig,
	}

	c.clusterOutput = clusterOutput

	dependency.ClusterOutput = clusterOutputDTO

	return nil
}

func (c *ClusterEKS) modifyEKSSecurityGroup() error {
	modifyUniqueName := fmt.Sprintf("%s-modifySG", c.cluster.Name)
	_, err := vpc.NewSecurityGroupIngressRule(c.ctx, modifyUniqueName, &vpc.SecurityGroupIngressRuleArgs{
		CidrIpv4:        pulumi.String(types.PUBLIC_CIDR),
		FromPort:        pulumi.Int(443),
		ToPort:          pulumi.Int(443),
		IpProtocol:      pulumi.String("tcp"),
		SecurityGroupId: c.clusterOutput.VpcConfig.ClusterSecurityGroupId().Elem(),
	})

	return err
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

func generateKubeconfig(clusterEndpoint pulumi.StringOutput, certData pulumi.StringOutput, clusterName pulumi.StringOutput) pulumi.StringOutput {
	return pulumi.Sprintf(`{
        "apiVersion": "v1",
        "clusters": [{
            "cluster": {
                "server": "%s",
                "certificate-authority-data": "%s"
            },
            "name": "kubernetes",
        }],
        "contexts": [{
            "context": {
                "cluster": "kubernetes",
                "user": "aws",
            },
            "name": "aws",
        }],
        "current-context": "aws",
        "kind": "Config",
        "users": [{
            "name": "aws",
            "user": {
                "exec": {
                    "apiVersion": "client.authentication.k8s.io/v1beta1",
                    "command": "aws-iam-authenticator",
                    "args": [
                        "token",
                        "-i",
                        "%s",
                    ],
                },
            },
        }],
    }`, clusterEndpoint, certData, clusterName)
}

func deployPodIdentityAgent(ctx *pulumi.Context, clusterName string, clusterRegion string, clusterOutput *eks.Cluster) error {
	pod := struct {
		ClusterName   string
		ClusterRegion string
	}{
		ClusterName:   clusterName,
		ClusterRegion: clusterRegion,
	}

	const POD_IDENTITY_AGENT = `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: eks-pod-identity-agent
  namespace: default
  labels:
    app.kubernetes.io/name: eks-pod-identity-agent
    app.kubernetes.io/instance: release-name
    app.kubernetes.io/version: "0.1.6"
spec:
  updateStrategy:
    rollingUpdate:
      maxUnavailable: 10%
    type: RollingUpdate
  selector:
    matchLabels:
      app.kubernetes.io/name: eks-pod-identity-agent
      app.kubernetes.io/instance: release-name
  template:
    metadata:
      labels:
        app.kubernetes.io/name: eks-pod-identity-agent
        app.kubernetes.io/instance: release-name
    spec:
      priorityClassName: system-node-critical
      hostNetwork: true
      terminationGracePeriodSeconds: 30
      tolerations:
        - operator: Exists
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: kubernetes.io/os
                operator: In
                values:
                - linux
              - key: kubernetes.io/arch
                operator: In
                values:
                - amd64
                - arm64
              - key: eks.amazonaws.com/compute-type
                operator: NotIn
                values:
                - fargate
      initContainers:
        - name: eks-pod-identity-agent-init
          image: 602401143452.dkr.ecr.us-west-2.amazonaws.com/eks/eks-pod-identity-agent:0.1.10
          imagePullPolicy: Always
          command: ['/go-runner', '/eks-pod-identity-agent', 'initialize']
          securityContext:
            privileged: true
      containers:
        - name: eks-pod-identity-agent
          image: 602401143452.dkr.ecr.us-west-2.amazonaws.com/eks/eks-pod-identity-agent:0.1.10
          imagePullPolicy: Always
          command: ['/go-runner', '/eks-pod-identity-agent', 'server']
          args:
            - "--port"
            - "80"
            - "--cluster-name"
            - "{{ .ClusterName }}"
            - "--probe-port"
            - "2703"
          ports:
            - containerPort: 80
              protocol: TCP
              name: proxy
            - containerPort: 2703
              protocol: TCP
              name: probes-port
          env:
          - name: AWS_REGION
            value: "{{ .ClusterRegion }}"
          securityContext:
            capabilities:
              add:
                - CAP_NET_BIND_SERVICE
          resources:
            {}
          livenessProbe:
            failureThreshold: 3
            httpGet:
              host: localhost
              path: /healthz
              port: probes-port
              scheme: HTTP
            initialDelaySeconds: 30
            timeoutSeconds: 10
          readinessProbe:
            failureThreshold: 30
            httpGet:
              host: localhost
              path: /readyz
              port: probes-port
              scheme: HTTP
            initialDelaySeconds: 1
            timeoutSeconds: 10`

	tmpl, err := template.New("pod-identity-agent").Parse(POD_IDENTITY_AGENT)
	if err != nil {
		return err
	}

	var r bytes.Buffer
	if err := tmpl.Execute(&r, pod); err != nil {
		return err
	}

	_, err = pulumiyaml.NewConfigGroup(ctx, "identity-agent-deploy", &pulumiyaml.ConfigGroupArgs{
		YAML: []string{r.String()},
	}, pulumi.DependsOn([]pulumi.Resource{clusterOutput}))

	return err
}

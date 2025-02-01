package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"pulumi-eks/internal/service/shared"
	"pulumi-eks/internal/types"
	"pulumi-eks/pkg/generic"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	yamlv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type PODIdentity struct {
	ctx     *pulumi.Context
	cluster types.Cluster

	identity types.IdentityPodAgent

	roleMap   map[string]*iam.Role
	policyMap map[string][]*iam.Policy

	rolePolicyAttachmentList []*iam.RolePolicyAttachment
}

func NewPodIdentity(ctx *pulumi.Context, cluster types.Cluster, identity types.IdentityPodAgent) *PODIdentity {
	return &PODIdentity{
		ctx:      ctx,
		cluster:  cluster,
		identity: identity,
	}
}

func (p *PODIdentity) Run(dependency *types.InterServicesDependencies) error {
	steps := []func() error{
		func() error { return p.validate() },
		func() error { return p.deployIdentityPodAgent(dependency) },
		func() error { return p.createIdentityRoles(dependency) },
		func() error { return p.createAWSRolePolicyAttachment() },
		func() error { return p.createSelfManagedPolicies() },
		func() error { return p.createSelfManagedRolePolicyAttachment() },
		func() error { return p.createIdentityRelationships() },
		func() error { return p.createServiceAccounts() },
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

func (p *PODIdentity) validate() error {
	if !p.identity.Deploy {
		return types.ErrNotErrorServiceSkipped
	}
	return nil
}

func (p *PODIdentity) createIdentityRelationships() error {
	var policyAttachmentDependsOn = make([]pulumi.Resource, len(p.rolePolicyAttachmentList))

	for i := range p.rolePolicyAttachmentList {
		policyAttachmentDependsOn[i] = p.rolePolicyAttachmentList[i]
	}

	for i, relationship := range p.identity.Identities.Relationships {
		role := p.roleMap[relationship.RoleName]

		role.Name.ApplyT(func(roleName string) error {
			podAssociationUniqueName := fmt.Sprintf("%d-%s-sa", i, roleName)

			_, err := eks.NewPodIdentityAssociation(p.ctx, podAssociationUniqueName, &eks.PodIdentityAssociationArgs{
				ClusterName:    pulumi.String(p.cluster.Name),
				Namespace:      pulumi.String(relationship.Namespace),
				ServiceAccount: pulumi.String(roleName + "-sa"),
				RoleArn:        role.Arn,
			}, pulumi.DependsOn(policyAttachmentDependsOn))

			return err
		})
	}
	return nil
}

func (p *PODIdentity) createServiceAccounts() error {
	var policyAttachmentDependsOn = make([]pulumi.Resource, len(p.rolePolicyAttachmentList))

	for i := range p.rolePolicyAttachmentList {
		policyAttachmentDependsOn[i] = p.rolePolicyAttachmentList[i]
	}

	for i, relationship := range p.identity.Identities.Relationships {
		serviceAccountUniqueName := fmt.Sprintf(
			"%d-%s-%s",
			i, relationship.RoleName, relationship.Namespace,
		)

		_, err := v1.NewServiceAccount(p.ctx, serviceAccountUniqueName, &v1.ServiceAccountArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.StringPtr(relationship.RoleName + "-sa"),
				Namespace: pulumi.StringPtr(relationship.Namespace),
			},
		}, pulumi.DependsOn(policyAttachmentDependsOn))
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *PODIdentity) createSelfManagedPolicies() error {
	policyMap := make(map[string][]*iam.Policy)
	selfManagedPoliciesList := make([]*iam.Policy, 0)

	iamRoleList := generic.FromMapValueToList(p.roleMap)

	iamRoleListDependsOn := generic.ToPulumiResourceList(iamRoleList, func(r *iam.Role) pulumi.Resource {
		return r
	})

	for ri, attach := range p.identity.Identities.Roles {
		if attach.SelfManagedPoliciesPath == nil {
			continue
		}

		for pi, policyPath := range attach.SelfManagedPoliciesPath {

			file, err := os.ReadFile(policyPath)
			if err != nil {
				return err
			}

			policyNameBasePath := filepath.Base(policyPath)
			policyName, _, _ := strings.Cut(policyNameBasePath, ".")

			policyUniqueName := fmt.Sprintf("%d-%s-%d-policy-sm", ri, attach.RoleName, pi)
			policy, err := iam.NewPolicy(p.ctx, policyUniqueName, &iam.PolicyArgs{
				Name:   pulumi.StringPtr(policyName),
				Policy: pulumi.String(string(file)),
			}, pulumi.DependsOn(iamRoleListDependsOn))

			if err != nil {
				return err
			}

			selfManagedPoliciesList = append(selfManagedPoliciesList, policy)
		}

		if _, exists := policyMap[attach.RoleName]; !exists {
			policyMap[attach.RoleName] = selfManagedPoliciesList
		}
	}

	p.policyMap = policyMap
	return nil
}

func (p *PODIdentity) createSelfManagedRolePolicyAttachment() error {
	var attachOutputList []*iam.RolePolicyAttachment

	iamRoleList := generic.FromMapValueToList(p.roleMap)

	iamRoleListDependsOn := generic.ToPulumiResourceList(iamRoleList, func(r *iam.Role) pulumi.Resource {
		return r
	})

	for roleName, attach := range p.policyMap {
		for i, policy := range attach {

			attachUniqueName := fmt.Sprintf("%d-%s-attach-sm", i, roleName)
			attachOutput, err := iam.NewRolePolicyAttachment(p.ctx, attachUniqueName, &iam.RolePolicyAttachmentArgs{
				Role:      pulumi.StringPtr(roleName),
				PolicyArn: policy.Arn,
			}, pulumi.DependsOn(iamRoleListDependsOn))

			if err != nil {
				return err
			}

			attachOutputList = append(attachOutputList, attachOutput)
		}
	}

	p.rolePolicyAttachmentList = attachOutputList

	return nil
}

func (p *PODIdentity) createAWSRolePolicyAttachment() error {
	var attachOutputList []*iam.RolePolicyAttachment

	iamRoleList := generic.FromMapValueToList(p.roleMap)

	iamRoleListDependsOn := generic.ToPulumiResourceList(iamRoleList, func(r *iam.Role) pulumi.Resource {
		return r
	})

	for ri, attach := range p.identity.Identities.Roles {
		for pi, policy := range attach.AwsPolicies {

			role := p.roleMap[attach.RoleName]

			attachUniqueName := fmt.Sprintf("%d-%s-%d-attach-am", ri, attach.RoleName, pi)
			attachOutput, err := iam.NewRolePolicyAttachment(p.ctx, attachUniqueName, &iam.RolePolicyAttachmentArgs{
				PolicyArn: pulumi.String(policy),
				Role:      role,
			}, pulumi.DependsOn(iamRoleListDependsOn))

			if err != nil {
				return err
			}

			attachOutputList = append(attachOutputList, attachOutput)
		}
	}

	p.rolePolicyAttachmentList = attachOutputList
	return nil
}

func (p *PODIdentity) deployIdentityPodAgent(dependency *types.InterServicesDependencies) error {
	dependsOn := shared.RetrieveDependsOnList(dependency)

	provider, err := kubernetes.NewProvider(p.ctx, "kubernetes-provider-identity-pod", &kubernetes.ProviderArgs{
		Kubeconfig: dependency.ClusterOutput.KubeConfig,
	})

	if err != nil {
		return err
	}

	dependency.ClusterOutput.EKSCluster.Name.ApplyT(func(clusterName string) error {
		return p.identityPodAgent(provider, dependency, dependsOn, clusterName, p.cluster.Region)
	})

	return nil
}

func (p *PODIdentity) createIdentityRoles(dependency *types.InterServicesDependencies) error {
	dependsOn := shared.RetrieveDependsOnList(dependency)

	assumeRoleIdentityPolicy, err := json.Marshal(map[string]interface{}{
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

	policyJSON := string(assumeRoleIdentityPolicy)

	roleMap := make(map[string]*iam.Role, len(p.identity.Identities.Roles))

	for _, data := range p.identity.Identities.Roles {
		role, err := iam.NewRole(p.ctx, data.RoleName, &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(policyJSON),
			Name:             pulumi.String(data.RoleName),
		}, pulumi.DependsOn(dependsOn))

		if err != nil {
			return err
		}

		if _, exists := roleMap[data.RoleName]; !exists {
			roleMap[data.RoleName] = role
		}
	}

	p.roleMap = roleMap

	return nil
}

func (p *PODIdentity) identityPodAgent(provider *kubernetes.Provider, dependency *types.InterServicesDependencies, dependsOn []pulumi.Resource, clusterName string, clusterRegion string) error {
	identityPodAgent := struct {
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
	if err := tmpl.Execute(&r, identityPodAgent); err != nil {
		return err
	}

	resourceOutput, err := yamlv2.NewConfigGroup(p.ctx, "identity-agent-deploy", &yamlv2.ConfigGroupArgs{
		Yaml: pulumi.StringPtr(r.String()),
	}, pulumi.DependsOn(dependsOn), pulumi.Provider(provider))

	dependency.PodIdentityAgent = resourceOutput

	return err
}

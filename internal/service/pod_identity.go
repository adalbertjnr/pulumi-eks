package service

import (
	"bytes"
	"html/template"
	"pulumi-eks/internal/service/shared"
	"pulumi-eks/internal/types"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	yamlv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type PODIdentity struct {
	ctx     *pulumi.Context
	cluster types.Cluster

	identity types.IdentityPodAgent
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
		func() error { return p.createIdentities() },
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

func (p *PODIdentity) createIdentities() error {
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

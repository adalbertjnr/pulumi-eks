# pulumi-eks

### A pulumi project to create an AWS infrastructure to deploy apps.

**1. Networking:**

- Vpc
- 4 Subnets (2 public and 2 private)

```yaml
networking:
  name: apps-vpc
  cidrBlock: "10.0.0.0/16"
  subnets:
    - name: apps-subnet-1a-priv
      cidrBlock: "10.0.0.0/24"
      publicIpOnLaunch: false
      availabilityZone: "us-east-1a"
      tags:
        "kubernetes.io/role/internal-elb": 1
    - name: apps-subnet-1b-priv
      cidrBlock: "10.0.1.0/24"
      publicIpOnLaunch: false
      availabilityZone: "us-east-1b"
      tags:
        "kubernetes.io/role/internal-elb": 1

    - name: apps-subnet-1a-pub
      cidrBlock: "10.0.2.0/24"
      publicIpOnLaunch: true
      availabilityZone: "us-east-1a"
      tags:
        "kubernetes.io/role/elb": 1
    - name: apps-subnet-1b-pub
      cidrBlock: "10.0.3.0/24"
      publicIpOnLaunch: true
      availabilityZone: "us-east-1b"
      tags:
        "kubernetes.io/role/elb": 1
```

- 2 Nat gateways to route internal traffic to internet
- 1 Internet gateway to route traffic to internet

**2. EKS Cluster**

- **Cluster**

```yaml
cluster:
  name: cluster-test
  environment: dev
  region: us-east-1
  kubernetesVersion: "1.31"
  vpcId: apps-vpc
  subnets: ["apps-subnet-1a-pub", "apps-subnet-1b-pub"]
  securityGroups: ["sg-102930sdccc", "sg-102390c0s"]
```

- **NodeGroups** - A list of node groups to be created dynamically when needed

```yaml
nodeGroups:
  - name: ng-dev-test
    scalingConfig:
      minSize: 1
      desiredSize: 1
      maxSize: 1
    instanceType: t3.medium
    imageId: "ami-0181ca43ef1eba8ed"
    nodeLabels:
      apps: pulumi-app
```

- **OIDC Provider** created dynamically using the helmChartComponent block (the ideia is to use for helm charts when required which is the case of alb controller chart)

```yaml
withOidcProvider:
  create: true
  role:
    name: AWSLoadBalancerControllerRole
    awsPolicies: []
    selfManagedPoliciesPath: ["../policies/alb_policy/alb-policy.json"]
  serviceAccount:
    name: aws-load-balancer-controller
```

- **EKS Pod Identity** can be true or false, but the ideia is for gave restricted permissions for applications by namespaces (apps namespaces must be created first otherwise it will throw some error, unless it's the default namespace initially)

```yaml
identityPodAgent:
  deploy: true
  identities:
    roles:
      - roleName: eks-pod-example
        awsPolicies: ["arn:aws:iam::aws:policy/AmazonS3FullAccess"]
        selfManagedPoliciesPath:
          ["../policies/identity_pod_policies/example-policy.json"]
    relationships:
      - roleName: eks-pod-example
        namespace: default
      - roleName: eks-pod-example
        namespace: example
```

- **HelmCharts**
  - the first example is using the oidcProvider, which means it will create the role with the **AssumeRoleWithWebIdentity**, policy and serviceAccount restricted by namespace and the serviceAccount

```yaml
helmChartsComponentes:
  components:
    - name: aws-lb-controller
      chart: aws-load-balancer-controller
      skipCidrs: false
      version: "1.11.0"
      repository: "https://aws.github.io/eks-charts"
      namespace: "kube-system"
      withOidcProvider:
        create: true
        role:
          name: AWSLoadBalancerControllerRole
          awsPolicies: []
          selfManagedPoliciesPath: ["../policies/alb_policy/alb-policy.json"]
        serviceAccount:
          name: aws-load-balancer-controller
      setValues:
        clusterName: cluster-test-new
        controllerConfig:
          featureGates:
            SubnetsClusterTagCheck: false
        replicaCount: 1
        serviceAccount:
          create: false
          name: aws-load-balancer-controller
```

- the second example is installing a chart not using the oidcProvider

```yaml
- name: argo-cd
  chart: argo-cd
  createNamespace: true
  skipCidrs: false
  version: "7.7.22"
  repository: "https://argoproj.github.io/argo-helm"
  namespace: "argocd"
  setValues:
    server:
      ingress:
        enabled: true
        annotations:
          alb.ingress.kubernetes.io/certificate-arn: ""
          alb.ingress.kubernetes.io/scheme: internet-facing
          alb.ingress.kubernetes.io/target-type: ip
          alb.ingress.kubernetes.io/backend-protocol: HTTPS
          alb.ingress.kubernetes.io/ssl-policy: ELBSecurityPolicy-FS-1-2-Res-2020-10
          alb.ingress.kubernetes.io/ssl-redirect: "443"
          alb.ingress.kubernetes.io/group.name: my-app-group
          alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS": 443}]'
          kubernetes.io/ingress.class: alb
        ingressClassName: alb
        hostname: argocd.example.com
```

**to run:**

```
pulumi up -C cmd/ --stack main --yes
```

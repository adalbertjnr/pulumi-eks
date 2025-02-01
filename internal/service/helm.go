package service

import (
	"fmt"
	"os"
	"path/filepath"
	"pulumi-eks/internal/service/shared"
	"pulumi-eks/internal/types"
	"pulumi-eks/pkg/generic"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Extensions struct {
	ctx            *pulumi.Context
	helmComponents types.HelmChartsComponentes
}

func NewExtensions(ctx *pulumi.Context, components types.HelmChartsComponentes) *Extensions {
	return &Extensions{
		ctx:            ctx,
		helmComponents: components,
	}
}

func (e *Extensions) Run(dependency *types.InterServicesDependencies) error {
	return e.applyHelmCharts(dependency)
}

func (e *Extensions) applyHelmCharts(dependency *types.InterServicesDependencies) error {
	dependsOn := shared.RetrieveDependsOnList(dependency)

	provider, err := kubernetes.NewProvider(e.ctx, "kubernetes-provider-helm", &kubernetes.ProviderArgs{
		Kubeconfig: dependency.ClusterOutput.KubeConfig,
	})

	if err != nil {
		return err
	}

	for _, component := range e.helmComponents.Components {

		if err := e.withOidcProvider(component, dependency); err != types.ErrNotErrorDisabledOIDCProvider && err != nil {
			return err
		}

		helmValue, err := checkSetValuesValue(component.SetValues)
		if err != nil {
			return err
		}

		_, err = helmv4.NewChart(e.ctx, component.Name, &helmv4.ChartArgs{
			Name:      pulumi.String(component.Name),
			Chart:     pulumi.String(component.Name),
			Namespace: pulumi.String(component.Namespace),
			SkipCrds:  pulumi.BoolPtr(component.SkipCirds),
			Version:   pulumi.String(*component.Version),
			RepositoryOpts: helmv4.RepositoryOptsArgs{
				Repo: pulumi.String(component.Repository),
			},
			Values: pulumi.ToMap(helmValue),
		}, pulumi.DependsOn(dependsOn), pulumi.Provider(provider))

		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Extensions) withOidcProvider(component types.Components, dependency *types.InterServicesDependencies) error {
	if component.WithOIDCProvider == nil || !component.WithOIDCProvider.Create {
		return types.ErrNotErrorDisabledOIDCProvider
	}

	oidc := dependency.ClusterOutput.EKSCluster.Identities.Index(pulumi.Int(0)).Oidcs().Index(pulumi.Int(0)).Issuer()

	oidc.ApplyT(func(oidcIssuer *string) error {

		arp, err := createAssumeRoleWithWebIdentity(
			e.ctx,
			*oidcIssuer,
			component.Namespace,
			component.WithOIDCProvider.ServiceAccount.Name,
		)

		if err != nil {
			return err
		}

		role, err := iam.NewRole(e.ctx, component.WithOIDCProvider.OidcIAMRole.Name, &iam.RoleArgs{
			Name:             pulumi.String(component.WithOIDCProvider.OidcIAMRole.Name),
			AssumeRolePolicy: pulumi.String(arp),
		})
		if err != nil {
			return err
		}

		if err := e.createAndAttachSelfManagedPolicies(
			role,
			component,
		); err != nil {
			return err
		}

		if err := e.attachAWSPolicies(
			role,
			component,
		); err != nil {
			return err
		}

		return nil
	})

	return nil
}

func createAssumeRoleWithWebIdentity(ctx *pulumi.Context, oidcProvider, namespace, serviceAccount string) (string, error) {
	cloudAccountId, err := generic.GetCallerIdentity(ctx)
	if err != nil {
		return "", err
	}

	oidcProvider = strings.ReplaceAll(oidcProvider, "https://", "")

	policy := fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": {
        "Effect": "Allow",
        "Principal": {
				    "Federated": "arn:aws:iam::%s:oidc-provider/%s"
				},
        "Action": "sts:AssumeRoleWithWebIdentity",
        "Condition": {
            "StringEquals": {
						    "%s:aud": "sts.amazonaws.com",
						    "%s:sub": "system:serviceaccount:%s:%s"
						}
        }
    }
}`,
		cloudAccountId,
		oidcProvider,
		oidcProvider,
		oidcProvider,
		namespace,
		serviceAccount,
	)

	return policy, nil
}

func (e *Extensions) createAndAttachSelfManagedPolicies(role *iam.Role, component types.Components) error {
	smPolicies := component.WithOIDCProvider.OidcIAMRole.SelfManagedPoliciesPath
	if smPolicies == nil || len(smPolicies) == 0 {
		return nil
	}

	for i, policy := range smPolicies {

		policyBasePath := filepath.Base(policy)
		policyNameExt := cases.Title(language.English).String(policyBasePath)
		policyName, _, _ := strings.Cut(policyNameExt, ".")

		file, err := os.ReadFile(policy)
		if err != nil {
			return err
		}

		policyUniqueName := fmt.Sprintf("%d-%s-policy-sm", i, policyName)
		policyOutput, err := iam.NewPolicy(e.ctx, policyUniqueName, &iam.PolicyArgs{
			Name:   pulumi.StringPtr(policyName),
			Policy: pulumi.String(string(file)),
		}, pulumi.DependsOn([]pulumi.Resource{role}))

		if err != nil {
			return err
		}

		attachUniqueName := fmt.Sprintf("%d-%s-attach-sm", i, policyName)
		_, err = iam.NewRolePolicyAttachment(e.ctx, attachUniqueName, &iam.RolePolicyAttachmentArgs{
			Role:      role,
			PolicyArn: policyOutput.Arn,
		}, pulumi.DependsOn([]pulumi.Resource{role, policyOutput}))
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Extensions) attachAWSPolicies(role *iam.Role, component types.Components) error {
	awsPolicies := component.WithOIDCProvider.OidcIAMRole.AwsPolicies
	if awsPolicies == nil || len(awsPolicies) == 0 {
		return nil
	}

	for i, policy := range awsPolicies {

		attachUniqueName := fmt.Sprintf("%d-%s-attach", i, component.WithOIDCProvider.OidcIAMRole.Name)
		_, err := iam.NewRolePolicyAttachment(e.ctx, attachUniqueName, &iam.RolePolicyAttachmentArgs{
			Role:      role,
			PolicyArn: pulumi.String(policy),
		}, pulumi.DependsOn([]pulumi.Resource{role}))
		if err != nil {
			return err
		}
	}

	return nil
}

func checkSetValuesValue(setValues map[string]interface{}) (map[string]interface{}, error) {
	for key, value := range setValues {
		if nested, ok := value.(map[string]interface{}); ok {
			if _, err := checkSetValuesValue(nested); err != nil {
				return nil, err
			}
		}
		if strValue, ok := value.(string); ok && strings.Contains(strValue, "from-file=") {
			filePath := strings.Split(strValue, "=")
			content, err := readFileContent(filePath[1])
			if err != nil {
				return nil, err
			}

			setValues[key] = content
		}
	}

	return setValues, nil
}

func readFileContent(filePath string) (string, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(file), nil
}

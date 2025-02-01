package service

import (
	"pulumi-eks/internal/types"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type OIDC struct {
	ctx     *pulumi.Context
	cluster types.Cluster
}

func NewOIDCProvider(ctx *pulumi.Context, cluster types.Cluster) *OIDC {
	return &OIDC{
		ctx:     ctx,
		cluster: cluster,
	}
}

func (o *OIDC) Run(dependency *types.InterServicesDependencies) error {
	return o.deployOIDCProvider(dependency)
}

func (o *OIDC) deployOIDCProvider(dependency *types.InterServicesDependencies) error {
	oidc := dependency.ClusterOutput.EKSCluster.Identities.Index(pulumi.Int(0)).Oidcs().Index(pulumi.Int(0)).Issuer()
	// sha1 := dependency.ClusterOutput.EKSCluster.Identities.Index(pulumi.Int(0)).Oidcs().Index(pulumi.Int(0)).Issuer()

	_, err := iam.NewOpenIdConnectProvider(o.ctx, "openid-connect-provider-eks", &iam.OpenIdConnectProviderArgs{
		Url:             oidc.Elem().ToStringOutput(),
		ClientIdLists:   pulumi.ToStringArray([]string{"sts.amazonaws.com"}),
		ThumbprintLists: nil,
	})

	return err
}

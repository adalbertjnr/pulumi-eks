package generic

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func GetCallerIdentity(ctx *pulumi.Context) (string, error) {
	identity, err := aws.GetCallerIdentity(ctx, &aws.GetCallerIdentityArgs{})
	if err != nil {
		return "", err
	}

	return identity.AccountId, nil
}

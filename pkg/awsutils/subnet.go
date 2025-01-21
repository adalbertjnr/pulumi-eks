package awsutils

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func GetSubnetIDs(ctx *pulumi.Context, subnetNames []string) []string {
	subnetsResult, err := ec2.GetSubnets(ctx, &ec2.GetSubnetsArgs{
		Filters: []ec2.GetSubnetsFilter{
			{
				Name:   "tag:Name",
				Values: subnetNames,
			},
		},
	})

	if err != nil {
		return nil
	}

	return subnetsResult.Ids
}

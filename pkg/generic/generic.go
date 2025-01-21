package generic

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ToStringOutputList[T any](collection []T, transform func(T) pulumi.StringOutput) []pulumi.StringOutput {
	pulumiIDOutputList := make([]pulumi.StringOutput, len(collection))

	for i := range collection {
		pulumiIDOutputList[i] = transform(collection[i])
	}

	return pulumiIDOutputList
}

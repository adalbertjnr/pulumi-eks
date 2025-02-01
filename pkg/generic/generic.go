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

func ToPulumiResourceList[T any](collection []T, transform func(T) pulumi.Resource) []pulumi.Resource {
	pulumiResourceList := make([]pulumi.Resource, len(collection))

	for i := range collection {
		pulumiResourceList[i] = transform(collection[i])
	}

	return pulumiResourceList
}

func FromMapValueToList[T any](mapCollection map[string]T) []T {
	var valueList []T

	for _, value := range mapCollection {
		valueList = append(valueList, value)
	}

	return valueList
}

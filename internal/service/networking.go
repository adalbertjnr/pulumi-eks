package service

import (
	"fmt"
	"pulumi-eks/internal/types"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Networking struct {
	ctx        *pulumi.Context
	networking types.Networking

	vpc *ec2.Vpc

	networkingConfigMap NetworkingConfigMap
}

func NewNetworking(ctx *pulumi.Context, n types.Networking) *Networking {
	return &Networking{
		ctx:                 ctx,
		networking:          n,
		networkingConfigMap: make(NetworkingConfigMap, 0),
	}
}

func (v *Networking) Run(interServicesDependencies *types.InterServicesDependencies) error {
	steps := []func() error{
		func() error { return v.networkingVpc() },
		func() error { return v.networkingSubnets(interServicesDependencies) },
		func() error { return v.networkingInternetGateway() },
		func() error { return v.networkingEIPs() },
		func() error { return v.networkingNatGateway() },
		func() error { return v.networkingRouteTable() },
		func() error { return v.networkingRoutes() },
		func() error { return v.networkingRouteTableAndSubnets() },
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

type NetworkingConfigMap map[string]NetworkingConfig

type NetworkingConfig struct {
	Subnet                 *ec2.Subnet
	NatGatewayPublicSubnet *ec2.Subnet
	EIP                    *ec2.Eip
	Type                   types.SubnetType
	InternetGateway        *ec2.InternetGateway
	NatGateway             *ec2.NatGateway
	RouteTable             *ec2.RouteTable
	Route                  *ec2.Route
}

func (v *Networking) networkingVpc() error {
	vpc, err := ec2.NewVpc(v.ctx, v.networking.Name, &ec2.VpcArgs{
		Tags:               pulumi.StringMap{"Name": pulumi.String(v.networking.Name)},
		CidrBlock:          pulumi.String(v.networking.CidrBlock),
		EnableDnsSupport:   pulumi.Bool(true),
		EnableDnsHostnames: pulumi.Bool(true),
	})

	v.vpc = vpc

	return err
}

func (v *Networking) networkingSubnets(d *types.InterServicesDependencies) error {
	var publicSubnet []*ec2.Subnet
	var sharedSubnetsBetweenResources []*ec2.Subnet

	for _, subnet := range v.networking.Subnets {
		if !subnet.PublicIpOnLaunch {
			continue
		}

		subnetOutput, err := ec2.NewSubnet(v.ctx, subnet.Name, &ec2.SubnetArgs{
			VpcId:               v.vpc.ID(),
			CidrBlock:           pulumi.String(subnet.CidrBlock),
			Tags:                pulumi.StringMap{"Name": pulumi.String(subnet.Name)},
			MapPublicIpOnLaunch: pulumi.Bool(subnet.PublicIpOnLaunch),
			AvailabilityZone:    pulumi.String(subnet.AvailabilityZone),
		})

		if err != nil {
			return err
		}

		publicSubnet = append(publicSubnet, subnetOutput)
		v.networkingConfigMap[subnet.Name] = NetworkingConfig{
			Subnet: subnetOutput,
			Type:   types.PUBLIC_SUBNET,
		}

		sharedSubnetsBetweenResources = append(sharedSubnetsBetweenResources, subnetOutput)
	}

	for i, subnet := range v.networking.Subnets {
		if subnet.PublicIpOnLaunch {
			continue
		}

		subnetOutput, err := ec2.NewSubnet(v.ctx, subnet.Name, &ec2.SubnetArgs{
			VpcId:               v.vpc.ID(),
			CidrBlock:           pulumi.String(subnet.CidrBlock),
			Tags:                pulumi.StringMap{"Name": pulumi.String(subnet.Name)},
			MapPublicIpOnLaunch: pulumi.Bool(subnet.PublicIpOnLaunch),
			AvailabilityZone:    pulumi.String(subnet.AvailabilityZone),
		})

		if err != nil {
			return err
		}

		v.networkingConfigMap[subnet.Name] = NetworkingConfig{
			Subnet:                 subnetOutput,
			Type:                   types.PRIVATE_SUBNET,
			NatGatewayPublicSubnet: publicSubnet[i],
		}

		sharedSubnetsBetweenResources = append(sharedSubnetsBetweenResources, subnetOutput)
	}

	d.Subnets = sharedSubnetsBetweenResources

	return nil
}

func (v *Networking) networkingNatGateway() error {

	for i, subnetConfig := range v.networking.Subnets {

		if value, exists := v.networkingConfigMap[subnetConfig.Name]; exists && value.Type == types.PRIVATE_SUBNET {
			natUniqueName := fmt.Sprintf("%s-ngw-%d", v.networking.Name, i)

			natGatewayOutput, err := ec2.NewNatGateway(v.ctx, natUniqueName, &ec2.NatGatewayArgs{
				Tags:         pulumi.StringMap{"Name": pulumi.String(natUniqueName)},
				AllocationId: value.EIP.ID(),
				SubnetId:     value.NatGatewayPublicSubnet.ID(),
			})

			if err != nil {
				return err
			}

			value.NatGateway = natGatewayOutput
			v.networkingConfigMap[subnetConfig.Name] = value
		}

	}

	return nil
}

func (v *Networking) networkingEIPs() error {
	for i, subnetConfig := range v.networking.Subnets {
		eniUniqueName := fmt.Sprintf("%s-eni-%d", v.networking.Name, i)

		if config, exists := v.networkingConfigMap[subnetConfig.Name]; exists && config.Type == types.PRIVATE_SUBNET {

			natGatewayEIP, err := ec2.NewEip(v.ctx, eniUniqueName, &ec2.EipArgs{
				Tags: pulumi.StringMap{"Name": pulumi.String(eniUniqueName)},
			})

			if err != nil {
				return err
			}

			config.EIP = natGatewayEIP
			v.networkingConfigMap[subnetConfig.Name] = config
		}

	}

	return nil
}

func (v *Networking) networkingInternetGateway() error {
	igwUniqueName := fmt.Sprintf("%s-igw", v.networking.Name)

	internetGateway, err := ec2.NewInternetGateway(v.ctx, igwUniqueName, &ec2.InternetGatewayArgs{
		Tags:  pulumi.StringMap{"Name": pulumi.String(igwUniqueName)},
		VpcId: v.vpc.ID(),
	})

	if err != nil {
		return err
	}

	for name, config := range v.networkingConfigMap {
		config.InternetGateway = internetGateway

		v.networkingConfigMap[name] = config
	}

	return nil
}

func (v *Networking) networkingRouteTable() error {
	for i, subnetConfig := range v.networking.Subnets {
		rtUniqueName := fmt.Sprintf("%s-rt-%d", v.networking.Name, i)

		if config, exists := v.networkingConfigMap[subnetConfig.Name]; exists {

			routeTableOutput, err := ec2.NewRouteTable(v.ctx, rtUniqueName, &ec2.RouteTableArgs{
				VpcId:  v.vpc.ID(),
				Routes: ec2.RouteTableRouteArray{},
			})

			if err != nil {
				return err
			}

			config.RouteTable = routeTableOutput
			v.networkingConfigMap[subnetConfig.Name] = config
		}
	}

	return nil
}

func (v *Networking) networkingRoutes() error {
	for i, subnet := range v.networking.Subnets {
		if config, exists := v.networkingConfigMap[subnet.Name]; exists {

			routeUniqueName := fmt.Sprintf("%v-route-%d", subnet.Name, i)
			if config.Type == types.PRIVATE_SUBNET {
				routeOutput, err := ec2.NewRoute(v.ctx, routeUniqueName, &ec2.RouteArgs{
					RouteTableId:         config.RouteTable.ID(),
					DestinationCidrBlock: pulumi.String(types.PUBLIC_CIDR),
					NatGatewayId:         config.NatGateway.ID(),
				})

				if err != nil {
					return err
				}
				config.Route = routeOutput
				v.networkingConfigMap[subnet.Name] = config
				continue
			}

			routeOutput, err := ec2.NewRoute(v.ctx, routeUniqueName, &ec2.RouteArgs{
				RouteTableId:         config.RouteTable.ID(),
				DestinationCidrBlock: pulumi.String(types.PUBLIC_CIDR),
				GatewayId:            config.InternetGateway.ID(),
			})
			if err != nil {
				return err
			}
			config.Route = routeOutput
			v.networkingConfigMap[subnet.Name] = config
		}
	}

	return nil
}

func (v *Networking) networkingRouteTableAndSubnets() error {
	for i, subnet := range v.networking.Subnets {
		associationUniqueName := fmt.Sprintf("%s-asc-%d", v.networking.Name, i)

		if config, exists := v.networkingConfigMap[subnet.Name]; exists {
			_, err := ec2.NewRouteTableAssociation(v.ctx, associationUniqueName, &ec2.RouteTableAssociationArgs{
				RouteTableId: config.RouteTable.ID(),
				SubnetId:     config.Subnet.ID(),
			})

			if err != nil {
				return err
			}

		}
	}

	return nil
}

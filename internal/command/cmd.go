package command

import "pulumi-eks/internal/types"

type Command interface {
	Run(*types.InterServicesDependencies) error
}

func New() *CreateCommands {
	return &CreateCommands{}
}

type CreateCommands struct {
	commands []Command
}

func (i *CreateCommands) AddCommand(cmd ...Command) {
	i.commands = append(i.commands, cmd...)
}

func (i *CreateCommands) RunCommands(interServicesDependencies *types.InterServicesDependencies) error {
	for _, cmd := range i.commands {
		if err := cmd.Run(interServicesDependencies); err != nil {
			return err
		}
	}

	return nil
}

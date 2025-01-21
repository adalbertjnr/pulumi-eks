package command

type Command interface {
	Run() error
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

func (i *CreateCommands) RunCommands() error {
	for _, cmd := range i.commands {
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}

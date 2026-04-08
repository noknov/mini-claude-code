package command

// Command represents a slash command
type Command struct {
	Name        string
	Description string
	Handler     func(args []string) error
}

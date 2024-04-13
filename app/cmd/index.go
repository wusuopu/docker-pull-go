package cmd

type Cli struct {
	Pull PullCmd `cmd:"" help:"Pull Image"`
	Push PushCmd `cmd:"" help:"Push Image"`
}
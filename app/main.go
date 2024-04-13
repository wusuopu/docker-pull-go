package main

import (
	"os"
	"strconv"

	"github.com/alecthomas/kong"
	"main.go/cmd"
)

func main() {
	debug, err := strconv.ParseBool(os.Getenv("DEBUG"))
	if err != nil {
		debug = false
	}

	var cli cmd.Cli
	ctx := kong.Parse(
		&cli,
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
	  	Compact: true,
			Summary: true,
		}),
	)
	err = ctx.Run(debug)

	ctx.FatalIfErrorf(err)

}
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/marinoborges/kubectl-apf/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cmd.Execute(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

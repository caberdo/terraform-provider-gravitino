package main

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"
)

// providerAddress is the registry address Terraform uses to select this provider.
const providerAddress = "registry.terraform.io/caberdo/gravitino"

// version is replaced at build time by GoReleaser with
// -ldflags "-X main.version=<release>".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses the provider's command-line flags and serves the plugin. It is
// separate from main so the flag handling can be exercised without exiting the
// process.
func run(args []string) error {
	flags := flag.NewFlagSet("terraform-provider-gravitino", flag.ContinueOnError)
	debug := flags.Bool("debug", false, "run the provider with support for debuggers like delve")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	return providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: providerAddress,
		Debug:   *debug,
	})
}

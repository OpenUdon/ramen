package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: ramen <command>\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Commands:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  apply     execute approved plans through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  author    draft a native UWS/Ramen project from prompt-safe API context\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  convert   generate Ramen review scaffolding from supported source formats\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  force-unlock release a local Ramen state lock by exact holder token\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  graph     emit the native resource dependency graph\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  icot      interactively draft a native UWS/Ramen project from local API metadata\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  import    attach an existing resource identity to state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  init      create or migrate local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  plan      emit a static desired-state plan without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  refresh   read tracked resources and update state through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  run       execute approved imperative UWS runbooks through a trusted executor\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  show      inspect Ramen plan and approval artifacts\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  state     inspect local Ramen state\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  validate  validate a native UWS/Ramen project without mutation\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version   print version\n")
	}
	flag.Parse()

	command := "help"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch command {
	case "apply":
		runApplyCommand(ctx, flag.Args()[1:])
	case "author":
		runAuthorCommand(ctx, flag.Args()[1:])
	case "convert":
		runConvertCommand(ctx, flag.Args()[1:])
	case "force-unlock":
		runForceUnlockCommand(ctx, flag.Args()[1:])
	case "graph":
		runGraphCommand(ctx, flag.Args()[1:])
	case "icot":
		runICOTCommand(ctx, flag.Args()[1:])
	case "import":
		runImportCommand(ctx, flag.Args()[1:])
	case "init":
		runInitCommand(ctx, flag.Args()[1:])
	case "plan":
		runPlanCommand(ctx, flag.Args()[1:])
	case "refresh":
		runRefreshCommand(ctx, flag.Args()[1:])
	case "run":
		runRunCommand(ctx, flag.Args()[1:])
	case "show":
		runShowCommand(flag.Args()[1:])
	case "state":
		runStateCommand(ctx, flag.Args()[1:])
	case "validate":
		runValidateCommand(ctx, flag.Args()[1:])
	case "version":
		runVersionCommand(flag.Args()[1:])
	case "-h", "--help", "help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		flag.Usage()
		os.Exit(2)
	}
}

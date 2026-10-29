package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type publishOptions struct {
	AgentPublishOptions
	Type string
}

func parsePublishOptions(args []string) (publishOptions, error) {
	var o publishOptions
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			return o, flag.ErrHelp
		}
		return o, fmt.Errorf("usage: skillhub publish <directory-or-zip> [--type skill|agent] [flags]")
	}
	o.Path = args[0]
	f := flag.NewFlagSet("publish", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Type, "type", "skill", "skill or agent")
	f.StringVar(&o.Slug, "slug", "", "package slug")
	f.StringVar(&o.Version, "version", "", "version")
	f.StringVar(&o.Summary, "summary", "", "summary")
	f.StringVar(&o.Tags, "tags", "", "comma-separated tags")
	f.StringVar(&o.Changelog, "changelog", "", "changelog")
	f.StringVar(&o.Name, "name", "", "Agent display name")
	f.StringVar(&o.Kind, "kind", "", "single or suite")
	f.StringVar(&o.InstructionsFile, "instructions-file", "", "UTF-8 usage instructions file")
	if e := f.Parse(args[1:]); e != nil {
		return o, e
	}
	if f.NArg() != 0 {
		return o, fmt.Errorf("unexpected argument: %s", f.Arg(0))
	}
	if o.Type != "skill" && o.Type != "agent" {
		return o, fmt.Errorf("--type must be skill or agent")
	}
	if o.Type == "skill" && (o.Name != "" || o.Kind != "" || o.InstructionsFile != "") {
		return o, fmt.Errorf("--name, --kind and --instructions-file require --type agent")
	}
	return o, nil
}

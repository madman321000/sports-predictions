package main

import (
	"flag"
	"fmt"
	"github.com/madman321000/sports-predictions/internal/dataset"
	"io"
)

type options struct {
	scope       dataset.Scope
	action, out string
	allow       bool
}

func parseOptions(args []string, out io.Writer) (options, error) {
	var o options
	f := flag.NewFlagSet("data", flag.ContinueOnError)
	f.SetOutput(out)
	f.StringVar(&o.action, "action", "quality", "quality or export")
	f.StringVar(&o.scope.League, "league", "NBA", "NBA or NFL")
	f.IntVar(&o.scope.Season, "season", 0, "ESPN season year")
	f.IntVar(&o.scope.SeasonType, "season-type", 2, "2 regular season, 3 postseason")
	f.StringVar(&o.scope.From, "from", "", "first expected ESPN import date, YYYY-MM-DD")
	f.StringVar(&o.scope.To, "to", "", "last expected ESPN import date, YYYY-MM-DD")
	f.StringVar(&o.out, "out", "", "new export directory (export only)")
	f.BoolVar(&o.allow, "allow-incomplete", false, "explicitly export despite quality findings")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() != 0 {
		return o, fmt.Errorf("no positional arguments expected")
	}
	if err := o.scope.Validate(); err != nil {
		return o, err
	}
	if o.action != "quality" && o.action != "export" {
		return o, fmt.Errorf("action must be quality or export")
	}
	if o.action == "export" && o.out == "" {
		return o, fmt.Errorf("export requires -out")
	}
	if o.action == "quality" && (o.out != "" || o.allow) {
		return o, fmt.Errorf("out and allow-incomplete only apply to export")
	}
	return o, nil
}

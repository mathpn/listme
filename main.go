package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/akamensky/argparse"
	logging "github.com/op/go-logging"

	"github.com/mathpn/listme/pretty"
	"github.com/mathpn/listme/search"
)

var log = logging.MustGetLogger("listme")
var format = logging.MustStringFormatter(`%{color}%{level}%{color:reset}: %{message}`)
var tags = []string{"BUG", "FIXME", "XXX", "TODO", "HACK", "OPTIMIZE", "NOTE"}
var tagValRegex = regexp.MustCompile(`^(\w+)$`)

func validateTags(tags []string) error {
	for _, tag := range tags {
		match := tagValRegex.MatchString(tag)
		if !match {
			return fmt.Errorf("provided tags must be non-empty and contain only alphanumeric or underscore characters")
		}
	}
	return nil
}

func main() {
	parser := argparse.NewParser("listme", "Summarize you FIXME, TODO, XXX (and other tags) comments so you don't forget them.")
	path := parser.StringPositional(&argparse.Options{Help: "Path to folder or file to be searched. Search is recursive."})
	workers := parser.Int("w", "workers", &argparse.Options{Default: 128, Help: "number of search workers"})
	tags := parser.StringList("T", "tags", &argparse.Options{Default: tags, Validate: validateTags, Help: "tags to search for"})
	glob := parser.String("g", "glob", &argparse.Options{Default: "*.*", Help: "Glob pattern to filter files in the search. Use a single-quoted string. Example: '*.go'"})
	ageLimit := parser.Int("l", "age-limit", &argparse.Options{Default: 60, Help: "Age limit for commits in days. Commits older than this limit are marked"})
	fullPath := parser.Flag("F", "full-path", &argparse.Options{Help: "Print full absolute path of the files"})
	noAuthor := parser.Flag("A", "no-author", &argparse.Options{Help: "Do not print git author information"})
	noSummary := parser.Flag("S", "no-summary", &argparse.Options{Help: "Do not print summary box for each file"})
	bw := parser.Flag("b", "bw", &argparse.Options{Help: "Use black and white style"})
	plain := parser.Flag("p", "plain", &argparse.Options{Help: "Use plain style. Ideal for machine consumption. Used by default when redirecting the output"})
	warning := parser.Flag("v", "verbose", &argparse.Options{Help: "Add warning verbosity"})
	debug := parser.Flag("d", "debug", &argparse.Options{Help: "Add debug verbosity"})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}

	logging.SetFormatter(format)
	b := logging.NewLogBackend(os.Stdout, "", 0)
	bFormatter := logging.NewBackendFormatter(b, format)
	logging.SetBackend(bFormatter)
	logging.SetLevel(logging.ERROR, "")
	if *warning {
		logging.SetLevel(logging.WARNING, "")
	}
	if *debug {
		logging.SetLevel(logging.DEBUG, "")
	}

	style, err := pretty.GetStyle(*bw, *plain)
	if err != nil {
		panic(err)
	}

	params := search.NewSearchParams(
		*path, *tags, *workers, style, *ageLimit, *fullPath, *noSummary, *noAuthor, *glob,
	)
	search.Search(params)
}

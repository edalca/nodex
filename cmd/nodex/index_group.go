package main

import (
	"errors"
	"fmt"
	"io"
)

const indexUsage = "usage: nodex index generate|status|comments|declarations|show"

func dispatchIndex(inv invocation, args []string, getwd func() (string, error), stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, indexUsage)
		return errors.New("usage")
	}
	switch args[0] {
	case "generate", "status", "comments", "declarations", "show":
	default:
		fmt.Fprintf(stderr, "unknown index command %q\n", args[0])
		return errors.New("usage")
	}
	if args[0] == "show" {
		if len(args) == 1 {
			fmt.Fprintln(stderr, "index show requires an ID")
			return errors.New("usage")
		}
	} else if len(args) != 1 {
		fmt.Fprintf(stderr, "index %s takes no arguments\n", args[0])
		return errors.New("usage")
	}
	dirs, err := locations(inv, getwd)
	if err != nil {
		fmt.Fprintf(stderr, "nodex: %v\n", err)
		return err
	}
	switch args[0] {
	case "generate":
		nComments, nDecls, nSources, err := generate(dirs)
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		fmt.Fprintf(stdout, "generated %d %s and %d %s from %d %s\n",
			nComments, countNoun(nComments, "comment", "comments"),
			nDecls, countNoun(nDecls, "declaration", "declarations"),
			nSources, countNoun(nSources, "source file", "source files"))
		return nil
	case "comments":
		text, err := comments(dirs)
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		return writeStdout(stdout, stderr, text)
	case "declarations":
		text, err := declarations(dirs)
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		return writeStdout(stdout, stderr, text)
	case "status":
		text, err := status(dirs)
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		return writeStdout(stdout, stderr, text)
	case "show":
		text, err := show(dirs, args[1:])
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		return writeStdout(stdout, stderr, text)
	}
	fmt.Fprintf(stderr, "unknown index command %q\n", args[0])
	return errors.New("usage")
}

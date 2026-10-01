package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/edalca/nodex/internal/project"
	"github.com/edalca/nodex/internal/skill"
)

// userHomeDir resolves the current user's home directory for global skill
// installation. Tests replace it so a test does not write to a real home.
var userHomeDir = os.UserHomeDir

func dispatchSkill(inv invocation, args []string, getwd func() (string, error), stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: nodex skill targets|show|install|uninstall")
		return errors.New("usage")
	}
	switch args[0] {
	case "targets":
		if err := skillNoArgs("targets", args[1:]); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return errors.New("usage")
		}
		return writeStdout(stdout, stderr, formatSkillTargets())
	case "show":
		if err := skillNoArgs("show", args[1:]); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return errors.New("usage")
		}
		return writeStdout(stdout, stderr, string(skill.Document()))
	case "install", "uninstall":
		return skillChange(inv, args[0], args[1:], getwd, stderr)
	default:
		fmt.Fprintf(stderr, "unknown skill command %q\n", args[0])
		return errors.New("usage")
	}
}

func skillNoArgs(command string, args []string) error {
	positional := 0
	for _, arg := range args {
		if arg == "--" || strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown option %q", arg)
		}
		positional++
	}
	if positional > 0 {
		return fmt.Errorf("skill %s takes no arguments", command)
	}
	return nil
}

func skillChange(inv invocation, command string, args []string, getwd func() (string, error), stderr io.Writer) error {
	targetName, global, err := parseSkillOp(command, args)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return errors.New("usage")
	}
	if global && inv.hasRoot {
		fmt.Fprintln(stderr, "--root and --global cannot be combined")
		return errors.New("usage")
	}
	target, err := skill.Lookup(targetName)
	if err != nil {
		fmt.Fprintf(stderr, "nodex: %v\n", err)
		return err
	}
	base, place, err := skillBase(inv, global, getwd)
	if err != nil {
		fmt.Fprintf(stderr, "nodex: %v\n", err)
		return err
	}
	if command == "install" {
		err = skill.Install(base, target, place)
	} else {
		err = skill.Uninstall(base, target, place)
	}
	if err != nil {
		fmt.Fprintf(stderr, "nodex: %v\n", err)
		return err
	}
	return nil
}

func parseSkillOp(command string, args []string) (string, bool, error) {
	var positional []string
	global := false
	for _, arg := range args {
		if arg == "--" || strings.HasPrefix(arg, "-") {
			if arg != "--global" {
				return "", false, fmt.Errorf("unknown option %q", arg)
			}
			if global {
				return "", false, errors.New("--global was provided more than once")
			}
			global = true
			continue
		}
		positional = append(positional, arg)
	}
	if len(positional) != 1 {
		return "", false, fmt.Errorf("skill %s requires one target", command)
	}
	return positional[0], global, nil
}

func skillBase(inv invocation, global bool, getwd func() (string, error)) (string, skill.Place, error) {
	if global {
		home, err := userHomeDir()
		if err != nil {
			return "", 0, err
		}
		if home == "" {
			return "", 0, errors.New("user home is not set")
		}
		return home, skill.Home, nil
	}
	dir, err := locate(inv, getwd)
	if err != nil {
		return "", 0, err
	}
	opened, err := project.Open(dir)
	if err != nil {
		return "", 0, err
	}
	return opened.Root(), skill.Project, nil
}

func formatSkillTargets() string {
	targets := skill.Targets()
	names := make([]string, len(targets))
	for i, target := range targets {
		names[i] = target.Name()
	}
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, "\n") + "\n"
}

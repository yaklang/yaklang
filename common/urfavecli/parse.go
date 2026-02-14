package cli

import (
	"flag"
	"strings"
)

type iterativeParser interface {
	newFlagSet() (*flag.FlagSet, error)
	useShortOptionHandling() bool
	ignoreUnknownFlags() bool
}

// To enable short-option handling (e.g., "-it" vs "-i -t") we have to
// iteratively catch parsing errors. This way we achieve LR parsing without
// transforming any arguments. Otherwise, there is no way we can discriminate
// combined short options from common arguments that should be left untouched.
// Pass `shellComplete` to continue parsing options on failure during shell
// completion when, the user-supplied options may be incomplete.
func parseIter(set *flag.FlagSet, ip iterativeParser, args []string, shellComplete bool) error {
	for {
		err := set.Parse(args)
		if (!ip.useShortOptionHandling() && !ip.ignoreUnknownFlags()) || err == nil {
			if shellComplete {
				return nil
			}
			return err
		}

		errStr := err.Error()
		trimmed := strings.TrimPrefix(errStr, "flag provided but not defined: -")
		if errStr == trimmed {
			return err
		}

		argsWereSplit := false
		for i, arg := range args {
			if name := strings.TrimLeft(arg, "-"); name != trimmed {
				continue
			}

			if ip.useShortOptionHandling() && ip.ignoreUnknownFlags() && isSplittable(arg) {
				unconditionalSplit := splitShortOptionsUnconditionally(arg)
				validShortOpts := []string{}
				for _, opt := range unconditionalSplit {
					flagName := strings.TrimLeft(opt, "-")
					if set.Lookup(flagName) != nil {
						validShortOpts = append(validShortOpts, opt)
					}
				}
				args = append(args[:i], append(validShortOpts, args[i+1:]...)...)
				argsWereSplit = true
				break
			}

			var shortOpts []string
			if ip.useShortOptionHandling() {
				shortOpts = splitShortOptions(set, arg)
			} else {
				shortOpts = []string{arg}
			}

			if len(shortOpts) == 1 {
				if ip.ignoreUnknownFlags() {
					args = append(args[:i], args[i+1:]...)
					argsWereSplit = true
					break
				}
				return err
			}

			args = append(args[:i], append(shortOpts, args[i+1:]...)...)
			argsWereSplit = true
			break
		}

		// This should be an impossible to reach code path, but in case the arg
		// splitting failed to happen, this will prevent infinite loops
		if !argsWereSplit {
			return err
		}

		// Since custom parsing failed, replace the flag set before retrying
		newSet, err := ip.newFlagSet()
		if err != nil {
			return err
		}
		*set = *newSet
	}
}

func splitShortOptions(set *flag.FlagSet, arg string) []string {
	shortFlagsExist := func(s string) bool {
		for _, c := range s[1:] {
			if f := set.Lookup(string(c)); f == nil {
				return false
			}
		}
		return true
	}

	if !isSplittable(arg) || !shortFlagsExist(arg) {
		return []string{arg}
	}

	separated := make([]string, 0, len(arg)-1)
	for _, flagChar := range arg[1:] {
		separated = append(separated, "-"+string(flagChar))
	}

	return separated
}

func splitShortOptionsUnconditionally(arg string) []string {
	if !isSplittable(arg) {
		return []string{arg}
	}

	separated := make([]string, 0, len(arg)-1)
	for _, flagChar := range arg[1:] {
		separated = append(separated, "-"+string(flagChar))
	}

	return separated
}

func isSplittable(flagArg string) bool {
	return strings.HasPrefix(flagArg, "-") && !strings.HasPrefix(flagArg, "--") && len(flagArg) > 2
}

// Copyright (c) 2023 ScyllaDB.

package cmdutil

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/errors"
)

const (
	FlagLogLevelKey   = "loglevel"
	flagLogLevelUsage = "Set the level of log output (0-10)."
)

func NormalizeNameForEnvVar(name string) string {
	s := strings.ToUpper(name)
	s = strings.Replace(s, "-", "_", -1)
	return s
}

func ReadFlagsFromEnv(prefix string, cmd *cobra.Command) error {
	var errs []error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			// flags always take precedence over environment
			return
		}

		// See if there exists matching environment variable
		envVarName := NormalizeNameForEnvVar(prefix + f.Name)
		v, exists := os.LookupEnv(envVarName)
		if !exists {
			return
		}

		err := f.Value.Set(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("can't parse env var %q with value %q into flag %q: %v", envVarName, v, f.Name, err))
			return
		}

		f.Changed = true

		return
	})

	return errors.NewAggregate(errs)
}

type proxyFlag struct {
	parentFlag flag.Value
	flagType   string
}

func (v *proxyFlag) Set(value string) error {
	return v.parentFlag.Set(value)
}

func (v *proxyFlag) String() string {
	return v.parentFlag.String()
}

func (v *proxyFlag) Type() string {
	return v.flagType
}

// InstallKlog registers a "loglevel" flag which value propagates into "v" flag used by underlying logger.
func InstallKlog(cmd *cobra.Command) {
	vFlag := flag.CommandLine.Lookup("v")
	if vFlag == nil {
		panic("'v' flag is not installed")
	}

	logLevelFlagProxy := &proxyFlag{
		parentFlag: vFlag.Value,
		flagType:   "int32",
	}
	cmd.PersistentFlags().Var(logLevelFlagProxy, FlagLogLevelKey, flagLogLevelUsage)

	// v might not be registered in cobra yet
	if cmd.PersistentFlags().Lookup("v") == nil {
		cmd.PersistentFlags().Var(logLevelFlagProxy, "v", flagLogLevelUsage)
	}
	cmd.PersistentFlags().Lookup("v").Hidden = true
}

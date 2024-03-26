package main

import (
	"fmt"
	"os"

	"github.com/bitrise-io/go-android/gradle"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/env"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/sliceutil"
	"github.com/kballard/go-shellquote"
)

// Configs ...
type Configs struct {
	ProjectLocation      string `env:"project_location,dir"`
	HTMLResultDirPattern string `env:"report_path_pattern"`
	XMLResultDirPattern  string `env:"result_path_pattern"`
	Variant              string `env:"variant"`
	Module               string `env:"module"`
	Arguments            string `env:"arguments"`

	DeployDir     string `env:"BITRISE_DEPLOY_DIR"`
	TestResultDir string `env:"BITRISE_TEST_RESULT_DIR"`
}

var cmdFactory = command.NewFactory(env.NewRepository())
var logger = log.NewLogger()

func main() {
	var config Configs

	fmt.Println(config)
	if err := stepconf.Parse(&config); err != nil {
		failf("Process config: couldn't create step config: %v\n", err)
	}
	stepconf.Print(config)
	fmt.Println()

	command := createCommand(config)

	runTest(command)
}

func runTest(command command.Command) {
	var testErr error
	logger.Infof("Run test:")
	fmt.Println()

	logger.Donef("$ " + command.PrintableCommandArgs())

	fmt.Println()

	testErr = command.Run()
	if testErr != nil {
		logger.Errorf("Run: test task failed, error: %v", testErr)
	}
}

func createCommand(config Configs) command.Command {
	project, err := gradle.NewProject(config.ProjectLocation, cmdFactory)
	if err != nil {
		failf("Process config: failed to open project, error: %s", err)
	}

	testTask := project.GetTask("verifySnapshots")

	args, err := shellquote.Split(config.Arguments)
	if err != nil {
		failf("Process config: failed to parse arguments, error: %s", err)
	}

	variants, err := testTask.GetVariants(args...)
	if err != nil {
		failf("Run: failed to fetch variants, error: %s", err)
	}

	filteredVariants, err := filterVariants(config.Module, config.Variant, variants)
	if err != nil {
		failf("Run: failed to find buildable variants, error: %s", err)
	}

	logVariants(variants, filteredVariants)

	return testTask.GetCommand(filteredVariants, args...)
}

func logVariants(variants gradle.Variants, filteredVariants gradle.Variants) {
	logger.Infof("Variants:")
	fmt.Println()

	for module, variants := range variants {
		logger.Printf("%s:", module)
		for _, variant := range variants {
			if sliceutil.IsStringInSlice(variant, filteredVariants[module]) {
				logger.Donef("✓ %s", variant)
			} else {
				logger.Printf("- %s", variant)
			}
		}
	}
	fmt.Println()
}

func failf(f string, args ...interface{}) {
	logger.Errorf(f, args...)
	os.Exit(1)
}

func filterVariants(module, variant string, variantsMap gradle.Variants) (gradle.Variants, error) {
	// if module set: drop all the other modules
	if module != "" {
		v, ok := variantsMap[module]
		if !ok {
			return nil, fmt.Errorf("module not found: %s", module)
		}
		variantsMap = gradle.Variants{module: v}
	}
	// if variant not set: use all variants
	if variant == "" {
		return variantsMap, nil
	}
	filteredVariants := gradle.Variants{}
	for m, variants := range variantsMap {
		for _, v := range variants {
			fmt.Println(v)
			if v == variant {
				filteredVariants[m] = append(filteredVariants[m], v)
			}
		}
	}
	if len(filteredVariants) == 0 {
		return nil, fmt.Errorf("variant %s not found in any module", variant)
	}
	return filteredVariants, nil
}

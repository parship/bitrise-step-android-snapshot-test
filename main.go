package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bitrise-io/go-android/cache"
	"github.com/bitrise-io/go-android/gradle"
	utilscache "github.com/bitrise-io/go-steputils/cache"
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
	CacheLevel           string `env:"cache_level,opt[none,only_deps,all]"`
	IsDebug              bool   `env:"is_debug,opt[true,false]"`

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

	logger.EnableDebugLog(config.IsDebug)

	gradleProject, err := gradle.NewProject(config.ProjectLocation, cmdFactory)
	if err != nil {
		failf("Process config: failed to open project, error: %s", err)
	}

	testTask := gradleProject.GetTask("test")

	args, err := shellquote.Split(config.Arguments)
	if err != nil {
		failf("Process config: failed to parse arguments, error: %s", err)
	}

	logger.Infof("Variants:")
	fmt.Println()

	variants, err := testTask.GetVariants(args...)
	if err != nil {
		failf("Run: failed to fetch variants, error: %s", err)
	}

	filteredVariants, err := filterVariants(config.Module, config.Variant, variants)
	if err != nil {
		failf("Run: failed to find buildable variants, error: %s", err)
	}

	for module, variants := range variants {
		logger.Printf("%s:", module)
		for _, variant := range variants {
			if sliceutil.IsStringInSlice(variant, filteredVariants[module]) {
				logger.Donef("✓ %s", strings.TrimSuffix(variant, "UnitTest"))
			} else {
				logger.Printf("- %s", strings.TrimSuffix(variant, "UnitTest"))
			}
		}
	}
	fmt.Println()

	// started := time.Now()

	var testErr error

	logger.Infof("Run test:")
	testCommand := testTask.GetCommand(filteredVariants, args...)

	fmt.Println()
	logger.Donef("$ " + testCommand.PrintableCommandArgs())
	fmt.Println()

	testErr = testCommand.Run()
	if testErr != nil {
		logger.Errorf("Run: test task failed, error: %v", testErr)
	}

	// EXPORT

	if testErr != nil {
		os.Exit(1)
	}

	fmt.Println()
	logger.Infof("Collecting cache:")
	if warning := cache.Collect(config.ProjectLocation, utilscache.Level(config.CacheLevel), cmdFactory); warning != nil {
		logger.Warnf("%s", warning)
	}
	logger.Donef("  Done")
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
			if strings.ToLower(v) == strings.ToLower(variant+"UnitTest") {
				filteredVariants[m] = append(filteredVariants[m], v)
			}
		}
	}
	if len(filteredVariants) == 0 {
		return nil, fmt.Errorf("variant %s not found in any module", variant)
	}
	return filteredVariants, nil
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/gradle"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/env"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/sliceutil"
	"github.com/kballard/go-shellquote"
)

// Configs ...
type Configs struct {
	ProjectLocation         string `env:"project_location,dir"`
	HTMLResultDirPattern    string `env:"report_path_pattern"`
	XMLResultDirPattern     string `env:"result_path_pattern"`
	SnapshotDeltaDirPattern string `env:"delta_path_pattern"`
	Variant                 string `env:"variant"`
	Module                  string `env:"module"`
	Arguments               string `env:"arguments"`

	DeployDir     string `env:"BITRISE_DEPLOY_DIR"`
	TestResultDir string `env:"BITRISE_TEST_RESULT_DIR"`
}

var cmdFactory = command.NewFactory(env.NewRepository())
var logger = log.NewLogger()

func main() {
	config := createConfig()

	project := getProject(config)

	command := createCommand(config, project)

	started := time.Now()

	testErr := runTest(command)

	exportResult(project, config, started)

	// FINISH
	if testErr != nil {
		os.Exit(1)
	}
}

func exportResult(project gradle.Project, config Configs, started time.Time) {
	// HTML RESULTS
	fmt.Println()
	logger.Infof("Export HTML results:")
	fmt.Println()

	reports, err := getArtifacts(project, started, config.HTMLResultDirPattern, true, true)
	if err != nil {
		failf("Export outputs: failed to find reports, error: %v", err)
	}

	if err := exportArtifacts(config.DeployDir, reports); err != nil {
		failf("Export outputs: failed to export reports, error: %v", err)
	}

	// XML RESULTS
	fmt.Println()
	logger.Infof("Export XML results:")
	fmt.Println()

	results, err := getArtifacts(project, started, config.XMLResultDirPattern, true, true)
	if err != nil {
		failf("Export outputs: failed to find results, error: %v", err)
	}

	if err := exportArtifacts(config.DeployDir, results); err != nil {
		failf("Export outputs: failed to export results, error: %v", err)
	}

	// SNAPSHOT RESULTS
	fmt.Println()
	logger.Infof("Export Snapshot results:")
	fmt.Println()

	snapshotResult, err := getArtifacts(project, started, config.SnapshotDeltaDirPattern, true, true)
	if snapshotResult != nil {
		failf("Export outputs: failed to find results, error: %v", err)
	}

	if err := exportArtifacts(config.DeployDir, results); err != nil {
		failf("Export outputs: failed to export results, error: %v", err)
	}

}

func createConfig() Configs {
	var config Configs

	fmt.Println(config)
	if err := stepconf.Parse(&config); err != nil {
		failf("Process config: couldn't create step config: %v\n", err)
	}
	stepconf.Print(config)
	fmt.Println()

	return config
}

func runTest(command command.Command) error {
	var testErr error
	logger.Infof("Run test:")
	fmt.Println()

	logger.Donef("$ " + command.PrintableCommandArgs())

	fmt.Println()

	testErr = command.Run()
	if testErr != nil {
		logger.Errorf("Run: test task failed, error: %v", testErr)
	}

	return testErr
}

func getProject(config Configs) gradle.Project {
	project, err := gradle.NewProject(config.ProjectLocation, cmdFactory)
	if err != nil {
		failf("Process config: failed to open project, error: %s", err)
	}

	return project
}

func createCommand(config Configs, project gradle.Project) command.Command {
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

func workDirRel(pth string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Rel(wd, pth)
}

func getArtifacts(variantsMap gradle.Variants, proj gradle.Project, started time.Time, pattern string, includeModuleName bool, isDirectoryMode bool) (artifacts []gradle.Artifact, err error) {
	var a []gradle.Artifact

	for m, _ := range variantsMap {
		patternWithVolume := m + "/" + pattern
		fmt.Println("Checking: " + patternWithVolume)

		moduleA, _ := proj.FindDirs(started, patternWithVolume, includeModuleName)

		a = append(a, moduleA...)
	}

	return a, nil
}

func exportArtifacts(deployDir string, artifacts []gradle.Artifact) error {
	for _, artifact := range artifacts {
		artifact.Name += ".zip"
		exists, err := pathutil.IsPathExists(filepath.Join(deployDir, artifact.Name))
		if err != nil {
			return fmt.Errorf("failed to check path, error: %v", err)
		}

		if exists {
			timestamp := time.Now().Format("20060102150405")
			artifact.Name = fmt.Sprintf("%s-%s%s", strings.TrimSuffix(artifact.Name, ".zip"), timestamp, ".zip")
		}

		src := filepath.Base(artifact.Path)
		if rel, err := workDirRel(artifact.Path); err == nil {
			src = "./" + rel
		}

		logger.Printf("  Export [ %s => $BITRISE_DEPLOY_DIR/%s ]", src, artifact.Name)

		if err := artifact.ExportZIP(deployDir); err != nil {
			logger.Warnf("failed to export artifact (%s), error: %v", artifact.Path, err)
			continue
		}
	}
	return nil
}

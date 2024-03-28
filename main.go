package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	"github.com/ryanuber/go-glob"
)

// Configs ...
type Configs struct {
	ProjectLocation         string `env:"project_location,dir"`
	HTMLResultDirPattern    string `env:"report_path_pattern"`
	XMLResultDirPattern     string `env:"result_path_pattern"`
	SnapshotDeltaDirPattern string `env:"delta_path_pattern"`
	SnapshotTask            string `env:"task"`
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

	testTask := project.GetTask(config.SnapshotTask)

	args := getArgs(config)

	variants := getVariants(config, *testTask, args)

	if len(variants) > 0 {
		command := testTask.GetCommand(variants, args...)

		testErr := runTest(command)

		exportResult(config, variants)

		// FINISH
		if testErr != nil {
			os.Exit(1)
		}
	} else {
		logger.Errorf("No buildable variants found. Skipping snapshot tests!")
	}
}

func exportResult(config Configs, variantMap gradle.Variants) {
	// // HTML RESULTS
	// fmt.Println()
	// logger.Infof("Export HTML results:")
	// fmt.Println()

	// htmlArtifacts, _ := getArtifacts(config, variantMap, config.HTMLResultDirPattern)
	// if htmlArtifacts != nil {
	// 	exportArtifacts(config.DeployDir, htmlArtifacts)
	// }

	// // XML RESULTS
	// fmt.Println()
	// logger.Infof("Export XML results:")
	// fmt.Println()

	// xmlArtifacts, _ := getArtifacts(config, variantMap, config.XMLResultDirPattern)

	// for artifact := range xmlArtifacts {
	// 	fmt.Println(artifact)
	// }

	// if xmlArtifacts != nil {
	// 	exportArtifacts(config.DeployDir, xmlArtifacts)
	// }

	// // SNAPSHOT RESULTS
	// fmt.Println()
	// logger.Infof("Export Snapshot results:")
	// fmt.Println()

	// snapshotArtifacts, _ := getArtifacts(config, variantMap, config.SnapshotDeltaDirPattern)
	// if snapshotArtifacts != nil {
	// 	exportArtifacts(config.DeployDir, snapshotArtifacts)
	// }

	// Test Addon
	if config.TestResultDir != "" {
		// Test Addon is turned on
		fmt.Println()
		logger.Infof("Export XML results for test addon:")
		fmt.Println()

		var xmlArtifacts []gradle.Artifact

		for m, variants := range variantMap {
			modulePath := strings.Replace(m, ":", "/", -1)
			pathToModule := config.ProjectLocation + "/" + modulePath + "/"

			for _, variant := range variants {
				xmlPath := pathToModule + config.XMLResultDirPattern + "/test" + variant + "UnitTest"

				artifacts, _ := findArtifacts(xmlPath, "*.xml", true)
				xmlArtifacts = append(xmlArtifacts, artifacts...)
			}

			failurePath := pathToModule + config.SnapshotDeltaDirPattern

			deltaArtifacts, _ := findArtifacts(failurePath, "delta-*.png", true)

			fmt.Println(deltaArtifacts)
		}

		lastOtherDirIdx := -1
		for _, artifact := range xmlArtifacts {
			lastOtherDirIdx = tryExportTestAddonArtifact(artifact.Path, config.TestResultDir, lastOtherDirIdx)
		}
	}
}

func tryExportTestAddonArtifact(artifactPth, outputDir string, lastOtherDirIdx int) int {
	dir := getExportDir(artifactPth)

	if dir == OtherDirName {
		// start indexing other dir name, to avoid overrideing it
		// e.g.: other, other-1, other-2
		lastOtherDirIdx++
		if lastOtherDirIdx > 0 {
			dir = dir + "-" + strconv.Itoa(lastOtherDirIdx)
		}
	}

	if err := ExportArtifact(artifactPth, outputDir, dir); err != nil {
		logger.Warnf("Failed to export test results for test addon: %s", err)
	} else {
		src := artifactPth
		if rel, err := workDirRel(artifactPth); err == nil {
			src = "./" + rel
		}
		logger.Printf("  Export [%s => %s]", src, filepath.Join("$BITRISE_TEST_RESULT_DIR", dir, filepath.Base(artifactPth)))
	}
	return lastOtherDirIdx
}

func createConfig() Configs {
	var config Configs

	if err := stepconf.Parse(&config); err != nil {
		failf("Process config: couldn't create step config: %v\n", err)
	}
	stepconf.Print(config)
	fmt.Println()

	return config
}

func getVariants(config Configs, task gradle.Task, args []string) gradle.Variants {
	variants, err := task.GetVariants(args...)
	if err != nil {
		failf("Run: failed to fetch variants, error: %s", err)
	}

	filteredVariants, err := filterVariants(config.Module, config.Variant, variants)
	if err != nil {
		failf("Run: failed to find buildable variants, error: %s", err)
	}

	logVariants(variants, filteredVariants)

	return filteredVariants
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

func getArgs(config Configs) []string {
	args, err := shellquote.Split(config.Arguments)
	if err != nil {
		failf("Process config: failed to parse arguments, error: %s", err)
	}

	return args
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
			if strings.ToLower(v) == strings.ToLower(variant) {
				filteredVariants[m] = append(filteredVariants[m], v)
			}
		}
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

func getArtifacts(config Configs, variantsMap gradle.Variants, pattern string) (artifacts []gradle.Artifact, err error) {
	var a []gradle.Artifact

	for module := range variantsMap {
		modulePath := strings.Replace(module, ":", "/", -1)
		fullPath := config.ProjectLocation + "/" + modulePath + "/" + pattern
		moduleName := strings.Replace(module, ":", "-", -1)

		name, err := extractArtifactName(config.ProjectLocation, fullPath, moduleName)
		if err == nil {
			a = append(a, gradle.Artifact{Name: name, Path: fullPath})
		}
	}

	return a, nil
}

func extractArtifactName(projectPath string, path string, module string) (string, error) {
	relPath, err := filepath.Rel(projectPath, path)
	if err != nil {
		return "", err
	}

	fileName := filepath.Base(relPath)

	fileName = module + "-" + fileName

	return fileName, nil
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

// FindArtifacts ...
func findArtifacts(folderPath string, pattern string, includeModuleInName bool) ([]gradle.Artifact, error) {
	var a []gradle.Artifact
	return a, filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warnf("failed to walk path: %s", err)
			return nil
		}

		if info.IsDir() || !glob.Glob(pattern, path) {
			return nil
		}

		name, err := extractArtifactName(folderPath, path, "test")
		if err != nil {
			return err
		}

		a = append(a, gradle.Artifact{Name: name, Path: path})
		return nil
	})
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bitrise-io/go-utils/command"
)

// OtherDirName is a directory name of non Android Unit test results
const OtherDirName = "other"

const (
	// ResultDescriptorFileName is the name of the test result descriptor file.
	ResultDescriptorFileName = "test-info.json"
)

func generateTestInfoFile(dir string, data []byte) error {
	f, err := os.Create(filepath.Join(dir, ResultDescriptorFileName))
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	return nil
}

// ExportArtifact exports artifact found at path in directory uniqueDir,
// rooted at baseDir.
func ExportArtifact(path, baseDir, uniqueDir string) error {
	exportDir := filepath.Join(baseDir, uniqueDir)

	if err := os.MkdirAll(exportDir, os.ModePerm); err != nil {
		return fmt.Errorf("skipping artifact (%s): could not ensure unique export dir (%s): %s", path, exportDir, err)
	}

	if _, err := os.Stat(filepath.Join(exportDir, ResultDescriptorFileName)); os.IsNotExist(err) {
		m := map[string]string{"test-name": uniqueDir}
		data, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("create test info descriptor: json marshal data (%s): %s", m, err)
		}
		if err := generateTestInfoFile(exportDir, data); err != nil {
			return fmt.Errorf("create test info descriptor: generate file: %s", err)
		}
	}

	name := filepath.Base(path)
	if err := command.CopyFile(path, filepath.Join(exportDir, name)); err != nil {
		return fmt.Errorf("failed to export artifact (%s), error: %v", name, err)
	}
	return nil
}

func getExportDir(artifactPath string) string {
	dir, err := getVariantDir(artifactPath)
	if err != nil {
		return OtherDirName
	}

	return dir
}

// getVariantDir returns the unique subdirectory inside the test addon export directory for a given artifact.
func getVariantDir(path string) (string, error) {
	parts := strings.Split(path, "/")

	i := indexOfTestResultsDirName(parts)
	if i == -1 {
		return "", fmt.Errorf("path (%s) does not contain 'test-results' folder", path)
	}

	variant, err := parseVariantName(parts, i)
	if err != nil {
		return "", fmt.Errorf("failed to parse variant name: %s", err)
	}

	module, err := parseModuleName(parts, i)
	if err != nil {
		return "", fmt.Errorf("failed to parse module name: %s", err)
	}

	return module + "-" + variant, nil
}

// indexOfTestResultsDirName finds the index of "test-results" in the given path parts, othervise returns -1
func indexOfTestResultsDirName(pthParts []string) int {
	// example: ./app/build/test-results/testDebugUnitTest/TEST-sample.results.test.multiple.bitrise.com.multipletestresultssample.UnitTest0.xml
	for i, part := range pthParts {
		if part == "test-results" {
			return i
		}
	}
	return -1
}

func parseVariantName(pthParts []string, testResultsPartIdx int) (string, error) {
	// example: ./app/build/test-results/testDebugUnitTest/TEST-sample.results.test.multiple.bitrise.com.multipletestresultssample.UnitTest0.xml
	if testResultsPartIdx+1 > len(pthParts) {
		return "", fmt.Errorf("unknown path (%s): Local Unit Test task output dir should follow the test-results part", filepath.Join(pthParts...))
	}

	taskOutputDir := pthParts[testResultsPartIdx+1]
	if !strings.HasPrefix(taskOutputDir, "test") || !strings.HasSuffix(taskOutputDir, "UnitTest") {
		return "", fmt.Errorf("unknown path (%s): Local Unit Test task output dir should match test*UnitTest pattern", filepath.Join(pthParts...))
	}

	variant := strings.TrimPrefix(taskOutputDir, "test")
	variant = strings.TrimSuffix(variant, "UnitTest")

	if len(variant) == 0 {
		return "", fmt.Errorf("unknown path (%s): Local Unit Test task output dir should match test<Variant>UnitTest pattern", filepath.Join(pthParts...))
	}

	return lowercaseFirstLetter(variant), nil
}

func lowercaseFirstLetter(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}

func parseModuleName(pthParts []string, testResultsPartIdx int) (string, error) {
	if testResultsPartIdx < 2 {
		return "", fmt.Errorf(`unknown path (%s): Local Unit Test task output dir should match <moduleName>/build/test-results`, filepath.Join(pthParts...))
	}
	return pthParts[testResultsPartIdx-2], nil
}

package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// utility to validate pre-requisites for deploying
func ValidateRequirements(buildPath string, siteName string) {
	// check for pull-secret.json
	if _, err := os.Stat(fmt.Sprintf("%s/pull-secret.json", buildPath)); os.IsNotExist(err) {
		log.Fatalf("Error, no valid pull-secret.json found in %s\n", buildPath)
	}

	// check for ssh key , and generate if it does not exist
	if _, err := os.Stat(fmt.Sprintf("%s/id_rsa.pub", buildPath)); os.IsNotExist(err) {
		log.Printf("No SSH public key (id_rsa.pub) found in %s. Generating keypair.\n", buildPath)

		var envVars []string
		ExecuteCommand("", envVars, true, true, "/bin/bash", "-c", fmt.Sprintf("ssh-keygen -b 2048 -q -N '' -f %s/id_rsa -C user@example.com", buildPath))
	}

	// check if requirements folder exist
	requirementsFolder := fmt.Sprintf("%s/%s/requirements", buildPath, siteName)
	if _, err := os.Stat(requirementsFolder); os.IsNotExist(err) {
		log.Fatalf("Error, requirements folder not found in %s\n", requirementsFolder)
	}

}

// utility to apply kustomize on a given directory
func ApplyKustomize(kustomizeBinary string, kustomizePath string) []byte {
	// retrieve executable path to inject env var
	ex, err := os.Executable()
	if err != nil {
		log.Fatal("Error retrieving the current running path")
	}
	pluginPath := filepath.Dir(ex)
	pluginPath, err = filepath.Abs(filepath.Join(pluginPath, "../plugins"))
	if err != nil {
		log.Fatalf("failed get plugin path: %v", err)
	}
	envVars := []string{fmt.Sprintf("XDG_CONFIG_HOME=%s", pluginPath)}
	out, _ := ExecuteCommand("", envVars, true, false, kustomizeBinary, "build", "--enable_alpha_plugins", "--reorder", "none", kustomizePath)

	return out
}

// utility to apply OC for a given output
func ApplyOc(ocBinary string, kubectlContent []byte, kubeconfigPath string, retryCount int, delay int) {
	// write content to be applied to temporary file
	tmpFile, err := ioutil.TempFile(os.TempDir(), "kubectl-")
	if err != nil {
		log.Fatal(fmt.Sprintf("Cannot create temporary file: %s", err))
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(kubectlContent)
	if err != nil {
		log.Fatalf("Error writing kubectl file: %s\n", err)
	}

	var envVars []string
	if len(kubeconfigPath) > 0 {
		envVars = []string{fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath)}
	}
	for i := 1; i <= retryCount; i++ {
		_, err := ExecuteCommand("", envVars, false, true, ocBinary, "apply", "-f", tmpFile.Name())

		if err == nil {
			// it is ok, stop the loop
			break
		} else {
			log.Println(string(err))
			// sleep and retry
			time.Sleep(time.Duration(delay) * time.Second)
		}
	}
}

// utility to execute a command and show the stdout and stderr output
func ExecuteCommand(directory string, envVars []string, failFatal bool, showOutput bool, name string, arg ...string) ([]byte, []byte) {
	cmd := exec.Command(name, arg...)

	// set additional modifiers
	if directory != "" {
		cmd.Dir = directory
	}
	cmd.Env = os.Environ()
	for _, envVar := range envVars {
		cmd.Env = append(cmd.Env, envVar)

	}
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()

	// show output of command
	if showOutput {
		log.Println(outb.String())
	}

	if err != nil {
		if failFatal {
			log.Fatalf("Error applying command %s (%s): %s - %s\n", name, arg, err, errb.String())
		} else {
			log.Printf("Error applying command %s (%s): %s - %s\n", name, arg, err, errb.String())
		}
	}
	return outb.Bytes(), errb.Bytes()
}

func CopyFile(sourcePath string, destinationPath string) error {
	sourceContents, err := ioutil.ReadFile(sourcePath)

	if err != nil {
		return err
	}

	err = ioutil.WriteFile(destinationPath, sourceContents, 0)

	if err != nil {
		return err
	}

	return nil
}

func ReplaceFileText(sourcePath string, oldText string, newText string) error {
	read, err := ioutil.ReadFile(sourcePath)

	if err != nil {
		return err
	}

	newContents := strings.Replace(string(read), oldText, newText, -1)

	err = ioutil.WriteFile(sourcePath, []byte(newContents), 0)

	if err != nil {
		return err
	}

	return nil
}

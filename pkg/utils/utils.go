package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// utility to validate pre-requisites for deploying
func ValidateRequirements(buildPath string, siteName string) {
	// check for pull-secret.json
	if _, err := os.Stat(fmt.Sprintf("%s/pull-secret.json", buildPath)); os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("Error, no valid pull-secret.json found in %s", buildPath))
		os.Exit(1)
	}

	// check for ssh key , and generate if it does not exist
	if _, err := os.Stat(fmt.Sprintf("%s/id_rsa.pub", buildPath)); os.IsNotExist(err) {
		log.Println(fmt.Sprintf("No SSH public key (id_rsa.pub) found in %s. Generating keypair.", buildPath))

		var envVars []string
		ExecuteCommand("", envVars, true, true, "/bin/bash", "-c", fmt.Sprintf("ssh-keygen -b 2048 -q -N '' -f %s/id_rsa -C user@example.com", buildPath))
	}

	// check if requirements folder exist
	requirementsFolder := fmt.Sprintf("%s/%s/requirements", buildPath, siteName)
	if _, err := os.Stat(requirementsFolder); os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("Error, requirements folder not found in %s", requirementsFolder))
		os.Exit(1)
	}

}

// utility to apply kustomize on a given directory
func ApplyKustomize(kustomizeBinary string, kustomizePath string) []byte {
	// retrieve executable path to inject env var
	ex, err := os.Executable()
	if err != nil {
		log.Fatal("Error retrieving the current running path")
		os.Exit(1)
	}
	exPath := filepath.Dir(ex)

	envVars := []string{fmt.Sprintf("XDG_CONFIG_HOME=%s/plugins", exPath)}
	out, _ := ExecuteCommand("", envVars, true, false, kustomizeBinary, "build", "--enable_alpha_plugins", "--reorder", "none", kustomizePath)

	return out
}

// utility to apply kubectl for a given output
func ApplyKubectl(kubectlBinary string, kubectlContent []byte, kubeconfigPath string) {
	// write content to be applied to temporary file
	tmpFile, err := ioutil.TempFile(os.TempDir(), "kubectl-")
	if err != nil {
		log.Fatal(fmt.Sprintf("Cannot create temporary file: %s", err))
		os.Exit(1)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(kubectlContent)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error writing kubectl file: %s", err))
		os.Exit(1)
	}

    envVars := []string{fmt.Sprintf("KUBECONFIG_PATH=%s", kubeconfigPath)}
	for i := 1; i <= 10; i++ {
		_, err := ExecuteCommand("", envVars, false, true, kubectlBinary, "apply", "-f", tmpFile.Name())

		if err == nil {
			// it is ok, stop the loop
			break
		} else {
			log.Println(err)
			// sleep and retry
			time.Sleep(60 * time.Second)
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
			log.Fatal("Error applying command: %s - %s", err, errb.String())
			os.Exit(1)
		} else {
			log.Println("Error applying command: %s - %s", err, errb.String())
		}
	}
	return outb.Bytes(), errb.Bytes()
}

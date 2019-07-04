package utils

import (
	"fmt"
	"log"
	"os"
	"os/exec"
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

		cmd := exec.Command("ssh-keygen", "-b", "2048", "-f", fmt.Sprintf("%s/id_rsa", buildPath), "-C", "user@example.com", "-q", "-N", "\"\"")
		err = cmd.Run()
		if err != nil {
			log.Fatal(fmt.Sprintf("Error generating ssh keypair: %s", err))
			os.Exit(1)
		}
	}

	// check if requirements folder exist
	requirementsFolder := fmt.Sprintf("%s/%s/requirements", buildPath, siteName)
	if _, err := os.Stat(requirementsFolder); os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("Error, requirements folder not found in %s", requirementsFolder))
		os.Exit(1)
	}

}

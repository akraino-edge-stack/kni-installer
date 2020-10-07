// Copyright © 2019 Red Hat <yroblamo@redhat.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	getter "github.com/hashicorp/go-getter"
	"github.com/spf13/cobra"
)

// BuildBinary is downloading the installer repo and building it
func BuildBinary(binPath string, installerRepo string, installerTag string) {
	repoURL := installerRepo
	if installerTag != "" {
		repoURL = fmt.Sprintf("%s?ref=%s", repoURL, installerTag)
	}

	installerPath := fmt.Sprintf("%s/go/src/github.com/openshift/installer", os.Getenv("HOME"))

	// remove if already exists, clone again
	os.RemoveAll(installerPath)

	client := &getter.Client{Src: repoURL, Dst: installerPath, Mode: getter.ClientModeAny}
	err := client.Get()

	if err != nil {
		log.Fatalf("Error downloading installer repo: %s\n", err)
	}

	log.Printf("Building installer on %s\n", installerPath)
	cmd := exec.Command("hack/build.sh")
	cmd.Dir = installerPath
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TAGS=libvirt")
	cmd.Env = append(cmd.Env, fmt.Sprintf("GOPATH=%s/go", os.Getenv("HOME")))

	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)
	cmd.Stdout = mw
	cmd.Stderr = mw

	err = cmd.Run()
	if err != nil {
		log.Fatalf("Error building binary: %s - %s\n", err, stdBuffer.String())
	}
	log.Println(stdBuffer.String())

	// copy the generated binary to the build directory
	cmd = exec.Command("cp", fmt.Sprintf("%s/bin/openshift-install", installerPath), binPath)
	err = cmd.Run()
	if err != nil {
		log.Fatalf("Error copying installer to buid path: %s\n", err)
	}
	log.Printf("Installer is available on %s/openshift-install\n", binPath)

}

// binaryCmd represents the binary command
var binaryCmd = &cobra.Command{
	Use:              "binary",
	Short:            "Command to build the openshift-install binary",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// retrieve config values and start generation
		installerRepo, _ := cmd.Flags().GetString("installer_repository")
		installerTag, _ := cmd.Flags().GetString("installer_tag")

		// bin path should exist, because "make build" should have been executed previously.
		binPath, _ := cmd.Flags().GetString("bin_path")
		os.Remove(binPath + "/openshift-install")
		BuildBinary(binPath, installerRepo, installerTag)
	},
}

func init() {
	rootCmd.AddCommand(binaryCmd)

	binaryCmd.Flags().StringP("installer_repository", "", "", "Url for the base github repository for openshift-installer")
	binaryCmd.MarkFlagRequired("installer_repository")
	binaryCmd.Flags().StringP("installer_tag", "", "master", "Specific tag for the openshift installer repository")
	binaryCmd.MarkFlagRequired("installer_tag")
	binaryCmd.Flags().StringP("bin_path", "", "", "Directory to use as build path.")

}

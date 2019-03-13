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
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// BuildImages will perform image generation
func BuildImages(buildPath string) {
	// start creating build directory for cosa
	cosaPath := fmt.Sprintf("%s/cosa_build", buildPath)
	os.RemoveAll(cosaPath)
	os.MkdirAll(cosaPath, 0775)

	// copy script to build directory
	scriptContent := `
#!/bin/bash

# init the build and proceed
cd /srv
coreos-assembler init https://github.com/yrobla/fedora-coreos-config --force

coreos-assembler fetch
coreos-assembler build
coreos-assembler buildextend-installer
`
	// create file with right permissions
	cosaFile := fmt.Sprintf("%s/cosa_build_image.sh", cosaPath)
	f, err := os.Create(cosaFile)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating cosa build script: %s", err))
		os.Exit(1)
	}
	defer f.Close()
	os.Chmod(cosaFile, 0775)

	// write content
	_, err = f.WriteString(scriptContent)
	f.Sync()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error writing cosa file: %s", err))
		os.Exit(1)
	}

	// run coreos-assembler
	cmd := exec.Command("podman", "run", "--rm", "--net=host", "-ti", "--privileged", "--userns=host", "-v", fmt.Sprintf("%s:/srv", cosaPath), "--workdir", "/srv", "quay.io/coreos-assembler/coreos-assembler:latest", "shell", "/srv/cosa_build_image.sh")
	cmd.Dir = buildPath

	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)
	cmd.Stdout = mw
	cmd.Stderr = mw

	err = cmd.Run()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error running coreos-assembler command: %s - %s", err, stdBuffer.String()))
		os.Exit(1)
	}
	log.Println(stdBuffer.String())

}

// imagesCmd represents the images command
var imagesCmd = &cobra.Command{
	Use:              "images",
	Short:            "Command to build the installer and deployment images (to be used on baremetal)",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if build path exists, create if not
		buildPath, _ := cmd.Flags().GetString("build_path")
		if len(buildPath) == 0 {
			// will generate a temporary directory
			buildPath, _ = ioutil.TempDir("/tmp", "kni")
		} else {
			// remove if exists, recreate
			os.RemoveAll(buildPath)
			os.MkdirAll(buildPath, 0775)
		}

		BuildImages(buildPath)

	},
}

func init() {
	rootCmd.AddCommand(imagesCmd)

	imagesCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")
}

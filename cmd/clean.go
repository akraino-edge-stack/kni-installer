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

	"github.com/spf13/cobra"
)

// cleanCmd represents the binary command
var cleanCmd = &cobra.Command{
	Use:              "clean",
	Short:            "Command to clean a previous cluster. It relies on metadata.json file",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if build path exists, if not fail
		buildPath, _ := cmd.Flags().GetString("build_path")

		// check if metadata exists
		metadataFile := fmt.Sprintf("%s/metadata.json", buildPath)
		if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
			log.Fatal(fmt.Sprintf("Metadata file %s does not exist. Stopping cleanup", metadataFile))
			os.Exit(1)
		} else {
			// launch cleanup command
			cmd := exec.Command("./openshift-install", "destroy", "cluster")
			cmd.Dir = buildPath
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, fmt.Sprintf("GOPATH=%s/go", os.Getenv("HOME")))

			var stdBuffer bytes.Buffer
			mw := io.MultiWriter(os.Stdout, &stdBuffer)
			cmd.Stdout = mw
			cmd.Stderr = mw

			err := cmd.Run()
			if err != nil {
				log.Fatal(fmt.Sprintf("Error cleaning cluster: %s - %s", err, stdBuffer.String()))
				os.Exit(1)
			}
			log.Println(stdBuffer.String())
			log.Println("Cluster cleaned successfully")
		}

	},
}

func init() {
	rootCmd.AddCommand(cleanCmd)

	cleanCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")

}

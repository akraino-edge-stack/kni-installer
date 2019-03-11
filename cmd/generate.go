// Copyright © 2019 NAME HERE <EMAIL ADDRESS>
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
	"io/ioutil"
	"os"

	"gerrit.akraino.org/kni/installer/pkg/generator"
	"github.com/spf13/cobra"
)

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:              "generate",
	Short:            "Command to generate manifests for a cluster",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// retrieve config values and start generation
		baseRepo, _ := cmd.Flags().GetString("base_repository")
		basePath, _ := cmd.Flags().GetString("base_path")
		installerPath, _ := cmd.Flags().GetString("installer_path")
		secretsRepository, _ := cmd.Flags().GetString("secrets_repository")
		settingsPath, _ := cmd.Flags().GetString("settings_path")

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

		// start generation process
		g := generator.New(baseRepo, basePath, installerPath, secretsRepository, settingsPath, buildPath)
		g.GenerateManifests()
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringP("base_repository", "", "", "Url for the base github repository for the blueprint")
	generateCmd.MarkFlagRequired("base_repository")
	generateCmd.Flags().StringP("base_path", "", "", "Path to the base config and manifests for the blueprint")
	generateCmd.MarkFlagRequired("base_path")
	generateCmd.Flags().StringP("installer_path", "", "https://github.com/openshift/installer/releases/download/v0.14.0/openshift-install-linux-amd64", "Path where openshift-install binary is stored")
	generateCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")

	generateCmd.Flags().StringP("secrets_repository", "", "", "Path to repository that contains secrets")
	generateCmd.MarkFlagRequired("secrets_repository")
	generateCmd.Flags().StringP("settings_path", "", "", "Path to repository that contains settings.yaml with definitions for the site")
	generateCmd.MarkFlagRequired("settings_path")
}

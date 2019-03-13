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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	getter "github.com/hashicorp/go-getter"
	"github.com/spf13/cobra"
)

// UmountDirectory will umount the ISO directory
func UmountDirectory(mountPath string) {
	cmd := exec.Command("sudo", "umount", mountPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error umounting directory: %s - %s", err, string(out)))
		os.Exit(1)
	}

	// remove mount directory
	os.RemoveAll(mountPath)
}

// BuildImages will perform image generation
func BuildImages(buildPath string, fcosURL string) {
	// first download metadata to retrieve the URL path
	metaPath := fmt.Sprintf("%s/meta.json", buildPath)
	client := &getter.Client{Src: fmt.Sprintf("%s/meta.json", fcosURL), Dst: metaPath}
	err := client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error downloading metadata: %s", err))
		os.Exit(1)
	}

	// parse the metadata to extract ISO path
	metaFile, err := os.Open(metaPath)
	defer metaFile.Close()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error opening metadata: %s", err))
		os.Exit(1)
	}
	jsonParser := json.NewDecoder(metaFile)
	metaSettings := &map[string]map[string]map[string]interface{}{}
	jsonParser.Decode(&metaSettings)
	var images map[string]interface{}

	if images = (*metaSettings)["images"]["iso"]; images == nil {
		log.Fatal("Malformed path to query images")
		os.Exit(1)
	}

	var imageName string
	if imageName = images["path"].(string); imageName == "" {
		log.Fatal("Error collecting iso path info")
		os.Exit(1)
	}

	isoURL := fmt.Sprintf("%s/%s", fcosURL, imageName)
	log.Println(isoURL)

	// start downloading ISO to build directory
	log.Println(fmt.Sprintf("Downloading image from %s", isoURL))
	isoPath := fmt.Sprintf("%s/fcos.iso", buildPath)
	client = &getter.Client{Src: isoURL, Dst: isoPath}
	err = client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error downloading installer ISO: %s", err))
		os.Exit(1)
	}

	// once there, mount it
	mountPath := fmt.Sprintf("%s/iso_mount", buildPath)
	os.RemoveAll(mountPath)
	os.MkdirAll(mountPath, 0775)

	// mount iso into that directory
	log.Println("Extracting image content")
	cmd := exec.Command("sudo", "mount", "-o", "loop", isoPath, mountPath)
	cmd.Dir = buildPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating manifests: %s - %s", err, string(out)))
		os.Exit(1)
	}

	// copy the images to the build directory
	cmd = exec.Command("cp", fmt.Sprintf("%s/images/vmlinuz", mountPath), buildPath)
	cmd.Dir = buildPath
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error copying vmlinuz: %s - %s", err, string(out)))
		UmountDirectory(mountPath)
		os.Exit(1)
	}

	cmd = exec.Command("cp", fmt.Sprintf("%s/images/initramfs.img", mountPath), buildPath)
	cmd.Dir = buildPath
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error copying initramfs: %s - %s", err, string(out)))
		UmountDirectory(mountPath)
		os.Exit(1)
	}
	UmountDirectory(mountPath)

	log.Println("Installer images are under: build/vmlinuz, build/initramfs.img")
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

		fcosURL, _ := cmd.Flags().GetString("fcos_url")
		if len(fcosURL) == 0 {
			log.Fatal("Missing FCOS URL for installer")
			os.Exit(1)
		}

		BuildImages(buildPath, fcosURL)
	},
}

func init() {
	rootCmd.AddCommand(imagesCmd)

	imagesCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")
	imagesCmd.Flags().StringP("fcos_url", "", "", "URL where to download the FCOS images metadata")
	imagesCmd.MarkFlagRequired("fcos_url")
}

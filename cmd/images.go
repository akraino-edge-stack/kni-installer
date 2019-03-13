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

// ExtractIsoFiles will extract vmlinuz and initramfs from a given ISO
func ExtractIsoFiles(isoPath string, buildPath string) {
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
	cmd = exec.Command("cp", fmt.Sprintf("%s/vmlinuz", mountPath), buildPath)
	cmd.Dir = buildPath
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error copying vmlinuz: %s - %s", err, string(out)))
		UmountDirectory(mountPath)
		os.Exit(1)
	}

	cmd = exec.Command("cp", fmt.Sprintf("%s/initramfs.img", mountPath), buildPath)
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

// GenerateInstallerImages will extract vmlinuz/ramdisk from modified FCOS iso
func GenerateInstallerImages(buildPath string) {
	// Generate build directory for cosa
	cosaPath := fmt.Sprintf("%s/cosa_build", buildPath)
	os.RemoveAll(cosaPath)
	os.MkdirAll(cosaPath, 0775)

	// generate installer file
	cosaBuildContent := `
#! /bin/bash
# init the build and proceed
cd /srv
coreos-assembler init https://github.com/yrobla/fedora-coreos-config --force

coreos-assembler build
coreos-assembler buildextend-installer	
`
	builderPath := fmt.Sprintf("%s/cosa_build_image.sh", cosaPath)
	f, err := os.Create(builderPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating installer script: %s", err))
		os.Exit(1)
	}
	_, err = f.WriteString(cosaBuildContent)
	f.Sync()
	f.Close()
	os.Chmod(builderPath, 0775)

	log.Println("Installing coreos-assembler and running generation script")
	cmd := exec.Command("podman", "run", "--rm", "--net=host", "-ti", "--privileged", "--userns=host", "-v", fmt.Sprintf("%s:/srv", cosaPath),
		"--workdir", "/srv", "quay.io/coreos-assembler/coreos-assembler:latest", "shell", "/srv/cosa_build_image.sh")
	cmd.Dir = buildPath

	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)
	cmd.Stdout = mw
	cmd.Stderr = mw

	err = cmd.Run()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error installing coreos-assembler: %s - %s", err, stdBuffer.String()))
		os.Exit(1)
	}
	log.Println(stdBuffer.String())

	// once the iso has been generated, extract vmlinuz/initramfs.img
	isoPath := fmt.Sprintf("%s/builds/latest/fedora-coreos-29.iso", cosaPath)
	if _, err := os.Stat(isoPath); os.IsNotExist(err) {
		// path/to/whatever does not exist
		log.Fatal("Final ISO image does not exist")
		os.Exit(1)
	}

	ExtractIsoFiles(isoPath, buildPath)

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

		// check version
		version, _ := cmd.Flags().GetString("version")
		if len(version) == 0 || (version != "maipo" && version != "ootpa") {
			log.Fatal("Version needs to be maipo or ootpa")
			os.Exit(1)
		}

		GenerateInstallerImages(buildPath)
	},
}

func init() {
	rootCmd.AddCommand(imagesCmd)

	imagesCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")
	imagesCmd.Flags().StringP("version", "", "", "Version of the images being generated (maipo, ootpa)")
}

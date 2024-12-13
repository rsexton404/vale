package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"archive/zip"
	"github.com/spf13/pflag"

	"github.com/errata-ai/vale/v3/internal/core"
)

// Style represents an externally-hosted style.
type Style struct {
	// User-provided fields.
	Author      string `json:"author"`
	Description string `json:"description"`
	Deps        string `json:"deps"`
	Feed        string `json:"feed"`
	Homepage    string `json:"homepage"`
	Name        string `json:"name"`
	URL         string `json:"url"`

	// Generated fields.
	HasUpdate bool `json:"has_update"`
	InLibrary bool `json:"in_library"`
	Installed bool `json:"installed"`
	Addon     bool `json:"addon"`
}

// Meta represents an installed style's meta data.
type Meta struct {
	Author      string   `json:"author"`
	Coverage    float64  `json:"coverage"`
	Description string   `json:"description"`
	Email       string   `json:"email"`
	Feed        string   `json:"feed"`
	Lang        string   `json:"lang"`
	License     string   `json:"license"`
	Name        string   `json:"name"`
	Sources     []string `json:"sources"`
	URL         string   `json:"url"`
	Vale        string   `json:"vale_version"`
	Version     string   `json:"version"`
}

func init() {
	pflag.BoolVar(&Flags.Remote, "mode-rev-compat", false,
		"prioritize local Vale configurations")
	pflag.StringVar(&Flags.Built, "built", "", "post-processed file path")

	Actions["install"] = install
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, file := range r.File {
		destPath := filepath.Join(dest, file.Name)
		if file.FileInfo().IsDir() {
			os.MkdirAll(destPath, os.ModePerm)
			continue
		}
		os.MkdirAll(filepath.Dir(destPath), os.ModePerm)

		outFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer outFile.Close()

		rc, err := file.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		_, err = io.Copy(outFile, rc)
		if err != nil {
			return err
		}
	}
	return nil
}

func fetch(src, dst string) error {
	resp, err := http.Get(src)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpfile, err := os.CreateTemp("", "temp.*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name())

	_, err = io.Copy(tmpfile, resp.Body)
	if err != nil {
		return err
	}

	return extractZip(tmpfile.Name(), dst)
}

func install(args []string, flags *core.CLIFlags) error {
	cfg, err := core.ReadPipeline(flags, false)
	if err != nil {
		return err
	}

	style := filepath.Join(cfg.StylesPath(), args[0])
	if core.IsDir(style) {
		os.RemoveAll(style) // Remove existing version
	}

	err = fetch(args[1], cfg.StylesPath())
	if err != nil {
		return sendResponse(
			fmt.Sprintf("Failed to install '%s'", args[1]),
			err)
	}

	return sendResponse(fmt.Sprintf(
		"Successfully installed '%s'", args[1]), nil)
}

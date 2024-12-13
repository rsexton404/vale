package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"archive/zip"
	cp "github.com/otiai10/copy"

	"github.com/errata-ai/vale/v3/internal/core"
)

var library = "https://raw.githubusercontent.com/errata-ai/styles/master/library.json"

func initPath(cfg *core.Config) error {
	// The first entry is always the default `StylesPath`.
	stylesPath := cfg.StylesPath()

	if !core.IsDir(stylesPath) {
		if err := os.MkdirAll(cfg.StylesPath(), os.ModePerm); err != nil {
			e := fmt.Errorf("unable to initialize StylesPath (value = '%s')", cfg.StylesPath())
			return core.NewE100("initPath", e)
		}
	}

	// Remove any existing .vale-config directory.
	err := os.RemoveAll(filepath.Join(stylesPath, core.PipeDir))
	if err != nil {
		return core.NewE100("initPath", err)
	}

	return nil
}

func readPkg(pkg, path string, idx int) error {
	lookup, err := getLibrary(path)
	if err != nil {
		return err
	}

	found := false
	for _, entry := range lookup {
		if pkg == entry.Name {
			found = true
			if err = download(pkg, entry.URL, path, idx); err != nil {
				return err
			}
		}
	}

	if !found {
		name := fileNameWithoutExt(pkg)
		if err = loadPkg(name, pkg, path, idx); err != nil {
			return err
		}
	}

	return nil
}

func loadPkg(name, urlOrPath, styles string, index int) error {
	if fileInfo, err := os.Stat(urlOrPath); err == nil {
		if fileInfo.IsDir() {
			return loadLocalPkg(name, urlOrPath, styles, index)
		}
		return loadLocalZipPkg(name, urlOrPath, styles, index)
	}
	return download(name, urlOrPath, styles, index)
}

func loadLocalPkg(name, pkgPath, styles string, index int) error {
	return installPkg(filepath.Dir(pkgPath), name, styles, index)
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

func loadLocalZipPkg(name, pkgPath, styles string, index int) error {
	dir, err := os.MkdirTemp("", name)
	if err != nil {
		return err
	}

	err = extractZip(pkgPath, dir)
	if err != nil {
		return err
	}

	return installPkg(dir, name, styles, index)
}

func download(name, url, styles string, index int) error {
	dir, err := os.MkdirTemp("", name)
	if err != nil {
		return err
	}

	if err = fetch(url, dir); err != nil {
		if strings.Contains(err.Error(), "unsupported protocol scheme") {
			err = fmt.Errorf("'%s' is not a valid URL or the local file doesn't exist", url)
		}
		return core.NewE100("download", err)
	}

	return installPkg(dir, name, styles, index)
}

func installPkg(dir, name, styles string, index int) error {
	root := filepath.Join(dir, name)
	path := filepath.Join(root, "styles")
	pipe := filepath.Join(styles, core.PipeDir)
	cfg := filepath.Join(root, ".vale.ini")

	if !core.IsDir(path) && !core.FileExists(cfg) {
		return moveAsset(name, dir, styles) // style-only
	}

	// StylesPath
	if core.IsDir(path) {
		if err := moveDir(path, styles); err != nil {
			return err
		}
		// $StylesPath/config
		//
		// NOTE: We treat this directory differently than the rest of the
		// entries on the path: we merge its contents with the existing
		// $StylesPath/config directory.
		for _, dir := range core.ConfigDirs {
			loc1 := filepath.Join(path, dir)
			if core.IsDir(loc1) {
				loc2 := filepath.Join(styles, dir)
				if err := moveDir(loc1, loc2); err != nil {
					return err
				}
			}
		}
	}

	// .vale.ini
	if core.FileExists(cfg) {
		pkgs, err := core.GetPackages(cfg)
		if err != nil {
			return err
		}

		for idx, pkg := range pkgs {
			if err = readPkg(pkg, styles, idx); err != nil {
				return err
			}
		}
		entry := fmt.Sprintf("%d-%s.ini", index, name)

		err = os.Rename(cfg, filepath.Join(root, entry))
		if err != nil {
			return err
		} else if err = moveAsset(entry, root, pipe); err != nil {
			return err
		}
	}

	return nil
}

func moveDir(old, new string) error { //nolint:predeclared
	files, err := os.ReadDir(old)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() || file.Name() != "config" {
			if err = moveAsset(file.Name(), old, new); err != nil {
				return err
			}
		}
	}

	return nil
}

func moveAsset(name, old, new string) error { //nolint:predeclared
	src := filepath.Join(old, name)
	dst := filepath.Join(new, name)

	if core.FileExists(dst) || core.IsDir(dst) {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}

	err := os.MkdirAll(new, os.ModePerm)
	if err != nil {
		return err
	}

	return cp.Copy(src, dst)
}

func getLibrary(_ string) ([]Style, error) {
	styles := []Style{}

	resp, err := fetchJSON(library)
	if err != nil {
		return styles, err
	} else if err = json.Unmarshal(resp, &styles); err != nil {
		return styles, err
	}

	return styles, err
}

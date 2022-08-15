package plugin

import (
	"encoding/json"
	"fmt"
	"github.com/wonderivan/logger"
	"os"
	"os/user"
	"path/filepath"
)

var ApiEndpoint = "https://api.gaydev.cc"
var AgentEndpoint = "http://127.0.0.1:35580"

type Manifest struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     int    `json:"version"`
	VersionName string `json:"versionName"`
	ExecCommand string `json:"execCommand"`
}

var HomeDir string
var PluginDir string

func Init() {
	current, err := user.Current()
	if err != nil {
		panic(err)
	}
	HomeDir = fmt.Sprintf("%s/.gaydev", current.HomeDir)
	PluginDir = fmt.Sprintf("%s/plugins", HomeDir)
}

func GetManifest(pluginId string) (Manifest, error) {
	thisPluginDir := fmt.Sprintf("%s/%s", PluginDir, pluginId)
	manifestFile := fmt.Sprintf("%s/manifest.json", thisPluginDir)
	manifest, err := readManifestFile(manifestFile)
	if err != nil {
		return manifest, err
	}
	if manifest.Id != pluginId {
		return manifest, fmt.Errorf("the plugin id is not match, the expected value is %s, but the actual value is %s", pluginId, manifest.Id)
	}
	return manifest, nil
}

func GetMyManifest() (Manifest, error) {
	manifestFile := fmt.Sprintf("%s/manifest.json", GetCurrentPath())
	manifest, err := readManifestFile(manifestFile)
	return manifest, err
}

func GetManifests() (map[string]Manifest, error) {
	plugins, err := os.ReadDir(PluginDir)
	if err != nil {
		return nil, err
	}
	manifests := make(map[string]Manifest, len(plugins))
	for _, pluginPath := range plugins {
		if pluginPath.IsDir() {
			manifest, err := GetManifest(pluginPath.Name())
			if err != nil {
				logger.Error(err)
				continue
			}
			manifests[manifest.Id] = manifest

		}
	}
	return manifests, nil
}

func GetCurrentPath() string {
	if ex, err := os.Executable(); err == nil {
		return filepath.Dir(ex)
	}
	return "./"
}

func readManifestFile(manifestFile string) (manifest Manifest, err error) {
	manifestJson, err := os.ReadFile(manifestFile)
	if err != nil {
		return manifest, err
	}
	err = json.Unmarshal(manifestJson, &manifest)
	if err != nil {
		return manifest, err
	}
	return manifest, nil
}

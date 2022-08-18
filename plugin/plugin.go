package plugin

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/wonderivan/logger"
	"github.com/xiwh/gaydev-agent-plugin/util"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

var ApiEndpoint = "https://api.gaydev.cc"
var AgentEndpoint = "http://127.0.0.1:35580"
var AgentAddr = "127.0.0.1:35580"
var Debug = false
var HomeDir string
var PluginDir string

type Manifest struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     int    `json:"version"`
	VersionName string `json:"versionName"`
	ExecCommand string `json:"execCommand"`
	Endpoint    string `json:"endpoint"`
}

func SetDebug() {
	Debug = true
}

func Init() {
	Debug = *flag.Bool("debug", Debug, "")
	current, err := user.Current()
	if err != nil {
		panic(err)
	}
	HomeDir = fmt.Sprintf("%s/.gaydev", current.HomeDir)
	PluginDir = fmt.Sprintf("%s/plugins", HomeDir)
	if !util.IsDir(PluginDir) {
		err := os.MkdirAll(PluginDir, 0755)
		if err != nil {
			logger.Error(err)
			panic(err)
		}
	}
}

func GetManifest(pluginId string) (Manifest, error) {
	thisPluginDir := fmt.Sprintf("%s/%s", PluginDir, pluginId)
	manifestFile := strings.Join([]string{thisPluginDir, string(os.PathSeparator), "manifest.json"}, "")
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
	manifestFile := strings.Join([]string{GetCurrentPath(), string(os.PathSeparator), "manifest.json"}, "")

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

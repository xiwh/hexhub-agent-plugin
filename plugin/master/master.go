package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gaydev-agent-plugin/util"
	uuid "github.com/satori/go.uuid"
	"github.com/wonderivan/logger"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"runtime"
)

var Token string
var HomeDir string
var PluginDir string
var pluginMap = make(map[string]Plugin)

const PluginStatusNotInstalled = 0
const PluginStatusDoownloading = 1
const PluginStatusDownloadFailed = 2
const PluginStatusInstallationFailed = 3
const PluginStatusNotStarted = 4
const PluginStatusStarted = 5

type Plugin struct {
	Id              string
	Name            string
	Description     string
	Version         int
	VersionName     string
	Dir             string
	ExecCommand     string
	Port            int
	Status          int
	DownloadProcess int
	ErrorMsg        string
	Endpoint        string
	Ctx             context.Context
}

type Manifest struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     int    `json:"version"`
	VersionName string `json:"versionName"`
	ExecCommand string `json:"execCommand"`
}

func Start() {
	Token = uuid.NewV4().String()
	user, err := user.Current()
	if err != nil {
		panic(err)
	}
	HomeDir = user.HomeDir
	PluginDir = fmt.Sprintf("%s/plugins", HomeDir)
}

func downloadPlugin(pluginId string, plugin *Plugin) error {
	url := "xxx"
	path, err := util.DownloadFile(url, func(total int64, current int64) {
		plugin.DownloadProcess = int(current / total * 100)
	})
	if err != nil {
		plugin.Status = PluginStatusDownloadFailed
		plugin.ErrorMsg = err.Error()
		return err
	}
	println(path)
	return nil
}

func startPlugin(pluginId string, plugin *Plugin) error {
	thisPluginDir := fmt.Sprintf("%s/%s", PluginDir, pluginId)
	manifestFile := fmt.Sprintf("%s/manifest.json", thisPluginDir)
	manifestJson, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	var pluginManifest Manifest
	err = json.Unmarshal(manifestJson, &pluginManifest)
	if err != nil {
		return err
	}
	if pluginManifest.Id != pluginId {
		return fmt.Errorf("the plugin id is not match, the expected value is %s, but the actual value is %s", pluginId, pluginManifest.Id)
	}
	ctx := context.Background()
	run(ctx, thisPluginDir, plugin, pluginManifest)
	plugin.Ctx = ctx
	plugin.Name = pluginManifest.Name
	plugin.Description = pluginManifest.Description
	plugin.Version = pluginManifest.Version
	plugin.VersionName = pluginManifest.VersionName
	plugin.Dir = thisPluginDir
	plugin.ExecCommand = pluginManifest.ExecCommand
	plugin.Version = pluginManifest.Version
	return nil
}

func run(ctx context.Context, thisPluginDir string, plugin *Plugin, pluginManifest Manifest) {
	var cmdStr string
	if runtime.GOOS == "windows" {
		cmdStr = fmt.Sprintf("%s/%s", thisPluginDir, pluginManifest.ExecCommand)
	} else {
		cmdStr = fmt.Sprintf("cd %s && ./%s", thisPluginDir, pluginManifest.ExecCommand)
	}
	cmd := exec.Command(cmdStr, "-token", Token, "-address", plugin.Endpoint)
	go func() {
		defer func() {
			plugin.Status = PluginStatusNotStarted
			err := recover()
			if err != nil {
				logger.Error(err)
			}
		}()
		err := cmd.Start()
		if err != nil {
			logger.Error(err)
		}
		plugin.Status = PluginStatusStarted
		err = cmd.Wait()
		if err != nil {
			logger.Error(err)
		}
		plugin.Status = PluginStatusNotStarted

	}()
	go func() {
		defer func() {
			err := recover()
			if err != nil {
				logger.Error(err)
			}
		}()
		<-ctx.Done()
		err := Post(plugin.Id, "kill", nil, nil)
		if err != nil {
			logger.Error(err)
		}
		err = cmd.Process.Kill()
		if err != nil {
			logger.Error(err)
		}
		plugin.Ctx = nil
	}()
}

func Post(pluginId string, uri string, req any, result any) error {
	plugin, ok := pluginMap[pluginId]
	if !ok {
		return fmt.Errorf("plugin %s does not exist", pluginId)
	}

	if plugin.Status != PluginStatusStarted {
		return fmt.Errorf("plugin %s not started", pluginId)
	}

	var reqBuf *bytes.Buffer
	if req != nil {
		reqData, err := json.Marshal(req)
		if err != nil {
			return err
		}
		reqBuf = bytes.NewBuffer(reqData)
	}

	client := &http.Client{}
	//生成要访问的url
	url := fmt.Sprintf("%s/%s", plugin.Endpoint, uri)

	//提交请求
	request, err := http.NewRequest("POST", url, reqBuf)

	//增加header选项
	request.Header.Add("Token", Token)
	request.Header.Add("PluginId", "main")
	request.Header.Add("Accept", "application/json")
	request.Header.Add("Content-Type", "application/json")

	if err != nil {
		return err
	}
	//处理返回结果
	response, _ := client.Do(request)
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error(err)
		}
	}(response.Body)
	if result == nil {
		return nil
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, result)
	return err
}

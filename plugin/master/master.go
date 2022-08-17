package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gaydev-agent-plugin/plugin"
	"gaydev-agent-plugin/util"
	uuid "github.com/satori/go.uuid"
	"github.com/wonderivan/logger"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

var Token string
var pluginMap = make(map[string]*PluginInfo)

const PluginStatusNotInstalled = 0
const PluginStatusDoownloading = 1
const PluginStatusDownloadFailed = 2
const PluginStatusInstallationFailed = 3
const PluginStatusNotStarted = 4
const PluginStatusStarted = 5

type PluginInfo struct {
	Id              string
	Name            string
	Description     string
	Version         int
	VersionName     string
	ExecCommand     string
	Port            int
	Status          int
	DownloadProcess int
	ErrorMsg        string
	Endpoint        string
	PluginDir       string
	Ctx             context.Context
}

func Start() {
	Token = uuid.NewV4().String()
	plugin.Init()
	manifests, err := plugin.GetManifests()
	if err != nil {
		panic(err)
	}
	for _, manifest := range manifests {
		initManifest(manifest)
	}
	http.HandleFunc("register-plugin", func(writer http.ResponseWriter, request *http.Request) {
		data, err := io.ReadAll(request.Body)
		if err != nil {
			logger.Error(err)
			writer.WriteHeader(500)
			return
		}
		var manifest *plugin.Manifest
		err = json.Unmarshal(data, manifest)
		if err != nil {
			logger.Error(err)
			writer.WriteHeader(500)
			return
		}
		pluginInfo := initManifest(*manifest)
		pluginInfo.Status = PluginStatusStarted
		if pluginInfo.Ctx != nil {
			pluginInfo.Ctx = context.Background()
			go func() {
				<-pluginInfo.Ctx.Done()
				err := Post(pluginInfo.Id, "kill", nil, nil)
				if err != nil {
					logger.Error(err)
				}
				pluginInfo.Ctx = nil
			}()
		}
	})
}

func initManifest(manifest plugin.Manifest) *PluginInfo {
	pluginInfo, ok := pluginMap[manifest.Id]
	if ok {
		pluginInfo.Version = manifest.Version
		pluginInfo.VersionName = manifest.VersionName
		pluginInfo.ExecCommand = manifest.ExecCommand
		pluginInfo.Description = manifest.Description
		pluginInfo.Endpoint = manifest.Endpoint
	} else {
		pluginInfo := &PluginInfo{
			Id:              manifest.Id,
			Name:            manifest.Name,
			Description:     manifest.Description,
			Version:         manifest.Version,
			VersionName:     manifest.VersionName,
			ExecCommand:     manifest.ExecCommand,
			Port:            -1,
			Status:          PluginStatusNotStarted,
			DownloadProcess: 0,
			ErrorMsg:        "",
			Endpoint:        manifest.Endpoint,
			PluginDir:       fmt.Sprintf("%s/%s", plugin.PluginDir, manifest.Id),
			Ctx:             nil,
		}
		pluginMap[manifest.Id] = pluginInfo
	}
	return pluginInfo
}

func RestartPlugin(pluginId string) error {
	_ = StopPlugin(pluginId)
	time.Sleep(500 * time.Millisecond)
	return StartPlugin(pluginId)
}

func StopPlugin(pluginId string) error {
	pluginInfo, ok := pluginMap[pluginId]
	if ok && pluginInfo.Ctx != nil {
		_, cancelFunc := context.WithCancel(pluginInfo.Ctx)
		cancelFunc()
		return nil
	}
	return fmt.Errorf("plugin %s is not started", pluginId)
}

func StartPlugin(pluginId string) error {
	manifest, err := plugin.GetManifest(pluginId)
	if err != nil {
		return err
	}
	pluginInfo := initManifest(manifest)
	pluginInfo.Ctx = context.Background()
	run(pluginInfo.Ctx, pluginInfo)
	return nil
}

func InstallPlugin(pluginId string, pluginInfo *PluginInfo) error {
	url := "xxx"
	path, err := util.DownloadFile(url, func(total int64, current int64) {
		pluginInfo.DownloadProcess = int(current / total * 100)
	})
	if err != nil {
		pluginInfo.Status = PluginStatusDownloadFailed
		pluginInfo.ErrorMsg = err.Error()
		return err
	}
	err = util.Unzip(path, pluginInfo.PluginDir, 0755)
	if err != nil {
		return err
	}
	//manifest, err := plugin.GetManifest(pluginId)
	//if err != nil {
	//	return err
	//}
	return nil
}

func run(ctx context.Context, plugin *PluginInfo) {
	var cmdStr string
	if runtime.GOOS == "windows" {
		cmdStr = fmt.Sprintf("%s/%s", plugin.PluginDir, plugin.ExecCommand)
	} else {
		cmdStr = fmt.Sprintf("cd %s && ./%s", plugin.PluginDir, plugin.ExecCommand)
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
		err := cmd.Process.Kill()
		if err != nil {
			logger.Error(err)
		}
		plugin.Ctx = nil
	}()
}

func Post(pluginId string, uri string, req any, result any) error {
	pluginInfo, ok := pluginMap[pluginId]
	if !ok {
		return fmt.Errorf("pluginInfo %s does not exist", pluginId)
	}

	if pluginInfo.Status != PluginStatusStarted {
		return fmt.Errorf("pluginInfo %s not started", pluginId)
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
	url := fmt.Sprintf("%s/%s", pluginInfo.Endpoint, uri)

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

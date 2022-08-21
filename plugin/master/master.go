package master

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	cmap "github.com/orcaman/concurrent-map/v2"
	uuid "github.com/satori/go.uuid"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/testutils"
	"github.com/wonderivan/logger"
	"github.com/xiwh/gaydev-agent-plugin/plugin"
	"github.com/xiwh/gaydev-agent-plugin/util"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var Token string
var pluginMap = cmap.New[*PluginInfo]()
var mForward *forward.Forwarder

const PluginStatusNotInstalled = 0
const PluginStatusDownloading = 1
const PluginStatusDownloadFailed = 2
const PluginStatusInstallationFailed = 3
const PluginStatusNotStarted = 4
const PluginStatusStarted = 5
const PluginStatusStarting = 6

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

type MasterRoute struct {
}

func (t MasterRoute) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	uri := req.URL.RequestURI()
	switch uri {
	case "":
	case "/":
	case "/ping":
		writer.WriteHeader(200)
		_, err := writer.Write([]byte("ok"))
		if err != nil {
			logger.Error(err)
		}
	case "/register-plugin":
		if !plugin.Debug && writer.Header().Get("Token") != Token {
			writer.WriteHeader(404)
			return
		}
		data, err := io.ReadAll(req.Body)
		if err != nil {
			logger.Error(err)
			writer.WriteHeader(500)
			return
		}
		var manifest plugin.Manifest
		err = json.Unmarshal(data, &manifest)
		if err != nil {
			logger.Error(err)
			writer.WriteHeader(500)
			return
		}
		pluginInfo := initManifest(manifest)
		if pluginInfo.Status == PluginStatusStarted {
			err := StopPlugin(pluginInfo.Id)
			if err != nil {
				logger.Error(err)
			}
		}
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
		writer.WriteHeader(200)
		_, err = writer.Write([]byte("ok"))
		if err != nil {
			logger.Error(err)
		}
	default:
		temp := uri[1:]
		idx := strings.IndexAny(temp, "/?")
		var pluginId string
		if idx != -1 {
			pluginId = temp[0:idx]
		} else {
			pluginId = temp[0:]
		}
		pluginInfo, ok := pluginMap.Get(pluginId)
		if ok {
			if pluginInfo.Status == PluginStatusStarted && pluginInfo.Endpoint != "" {
				redirectUrl := ""
				if idx != -1 {
					redirectUrl = temp[idx:]
				}
				req.URL = testutils.ParseURI(pluginInfo.Endpoint)
				req.RequestURI = redirectUrl
				req.Header.Add("Token", Token)
				mForward.ServeHTTP(writer, req)
				//http.Redirect(writer, req, redirectUrl, 301)
				return
			}
		}
		writer.WriteHeader(404)
	}
}

func Start() {
	Token = uuid.NewV4().String()
	logger.Info("token:%s", Token)
	plugin.Init()
	manifests, err := plugin.GetManifests()
	if err != nil {
		panic(err)
	}
	for _, manifest := range manifests {
		initManifest(manifest)
	}
	// Forwards incoming requests to whatever location URL points to, adds proper forwarding headers
	mForward, _ = forward.New()
	logger.Error(http.ListenAndServe(plugin.AgentAddr, new(MasterRoute)))
}

func initManifest(manifest plugin.Manifest) *PluginInfo {
	pluginInfo, ok := pluginMap.Get(manifest.Id)
	if ok {
		pluginInfo.Version = manifest.Version
		pluginInfo.VersionName = manifest.VersionName
		pluginInfo.ExecCommand = manifest.ExecCommand
		pluginInfo.Description = manifest.Description
		pluginInfo.Endpoint = manifest.Endpoint
	} else {
		pluginInfo = &PluginInfo{
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
			PluginDir:       strings.Join([]string{plugin.PluginDir, string(os.PathSeparator), manifest.Id}, ""),
			Ctx:             nil,
		}
		pluginMap.Set(manifest.Id, pluginInfo)
	}
	return pluginInfo
}

func RestartPlugin(pluginId string) error {
	err := StopPlugin(pluginId)
	//如果之前在启动中，那么等待500ms再重启，避免重启失败
	if err != nil {
		time.Sleep(500 * time.Millisecond)
	}
	return StartPlugin(pluginId)
}

func StopPlugin(pluginId string) error {
	pluginInfo, ok := pluginMap.Get(pluginId)
	if ok {
		if pluginInfo.Ctx != nil {
			_, cancelFunc := context.WithCancel(pluginInfo.Ctx)
			cancelFunc()
			return nil
		}
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

func run(ctx context.Context, pluginInfo *PluginInfo) {
	var cmdStr string
	if runtime.GOOS == "windows" {
		cmdStr = fmt.Sprintf("%s/%s", pluginInfo.PluginDir, pluginInfo.ExecCommand)
	} else {
		cmdStr = fmt.Sprintf("cd %s && ./%s", pluginInfo.PluginDir, pluginInfo.ExecCommand)
	}
	cmd := exec.Command(cmdStr, "-token", Token, "-address", pluginInfo.Endpoint)
	go func() {
		defer func() {
			pluginInfo.Status = PluginStatusNotStarted
			err := recover()
			if err != nil {
				logger.Error(err)
			}
		}()
		pluginInfo.Status = PluginStatusStarting
		err := cmd.Start()
		if err != nil {
			logger.Error(err)
		}
		err = cmd.Wait()
		if err != nil {
			logger.Error(err)
		}
		pluginInfo.Status = PluginStatusNotStarted
		if pluginInfo.Ctx != nil {
			_, cancelFunc := context.WithCancel(pluginInfo.Ctx)
			cancelFunc()
		}

	}()
	go func() {
		defer func() {
			err := recover()
			if err != nil {
				logger.Error(err)
			}
		}()
		<-ctx.Done()
		pluginInfo.Status = PluginStatusNotStarted
		err := cmd.Process.Kill()
		if err != nil {
			logger.Error(err)
		}
		pluginInfo.Ctx = nil
	}()
}

func Post(pluginId string, uri string, req any, result any) error {
	var pluginInfo *PluginInfo
	pluginInfo, ok := pluginMap.Get(pluginId)
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
	} else {
		reqBuf = bytes.NewBufferString("")
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
	response, err := client.Do(request)
	if err != nil {
		return err
	}
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

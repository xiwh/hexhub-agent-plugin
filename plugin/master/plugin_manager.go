package master

import (
	"fmt"
	"github.com/wonderivan/logger"
	"github.com/xiwh/hexhub-agent-plugin/plugin"
	"github.com/xiwh/hexhub-agent-plugin/util"
	httputil2 "github.com/xiwh/hexhub-agent-plugin/util/httputil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type VersionInfo struct {
	Manifest          plugin.Manifest `json:"manifest"`
	PluginId          string
	Version           int
	VersionName       string
	UpdateDescription string
	TotalSize         int64
	DownloadUrl       string
}

func initManifest(manifest plugin.Manifest) *PluginInfo {
	pluginInfo, ok := pluginMap.Get(manifest.PluginId)
	if ok {
		pluginInfo.Version = manifest.Version
		pluginInfo.VersionName = manifest.VersionName
		pluginInfo.ExecEnter = manifest.ExecEnter
		pluginInfo.Description = manifest.Description
		pluginInfo.Endpoint = manifest.Endpoint
		pluginInfo.AutoExit = manifest.AutoExit
		pluginInfo.DownloadedSize = 0
	} else {
		pluginInfo = &PluginInfo{
			Id:             manifest.PluginId,
			Name:           manifest.Name,
			Description:    manifest.Description,
			Version:        manifest.Version,
			VersionName:    manifest.VersionName,
			ExecEnter:      manifest.ExecEnter,
			AutoExit:       manifest.AutoExit,
			Status:         PluginStatusNotStarted,
			DownloadedSize: 0,
			ErrorMsg:       "",
			Endpoint:       manifest.Endpoint,
			PluginDir:      strings.Join([]string{plugin.PluginsDir, string(os.PathSeparator), manifest.PluginId}, ""),
		}
		pluginMap.Set(manifest.PluginId, pluginInfo)
	}
	return pluginInfo
}

func RestartPlugin(pluginId string) error {
	err := StopPlugin(pluginId)
	//如果之前在启动中，那么等待250ms再重启，避免重启失败
	if err == nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)
	return StartPlugin(pluginId)
}

func StopPlugin(pluginId string) error {
	pluginInfo, ok := pluginMap.Get(pluginId)
	if ok && pluginInfo.Status == PluginStatusRunning {
		err := Post(pluginId, "kill", nil, nil)
		pluginInfo.Status = PluginStatusNotStarted
		return err
	}
	return fmt.Errorf("plugin %s is not running", pluginId)
}

func StartPlugin(pluginId string) error {
	manifest, err := plugin.GetManifest(pluginId)
	if err != nil {
		return err
	}
	pluginInfo := initManifest(manifest)
	return run(pluginInfo)
}

func CheckUpdate(pluginId string) error {
	var latestInfo VersionInfo

	err := plugin.ApiGet("client/plugin/latest-version", map[string]string{
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"pluginId": pluginId,
	}, &latestInfo)
	if err != nil {
		return err
	}

	currentInfo, ok := pluginMap.Get(pluginId)
	//判断是否已下载并且版本是最新
	if !ok || currentInfo.Version < latestInfo.Manifest.Version {
		currentInfo.TotalSize = latestInfo.TotalSize
		return InstallPlugin(latestInfo.DownloadUrl, latestInfo.Manifest)
	}
	return nil
}

func UninstallPlugin(pluginId string) error {
	info, ok := pluginMap.Get(pluginId)
	if !ok {
		//即便是检测到未安装对应插件也把插件目录删除一下,防止安装过程中发生异常有残留文件跟安装程序发生冲突,导致永远无法重新安装陷入死循环
		_ = os.Remove(strings.Join([]string{plugin.PluginsDir, string(os.PathSeparator), pluginId}, ""))
		return nil
	}
	_ = StopPlugin(pluginId)
	time.Sleep(500 * time.Millisecond)
	err := os.Remove(info.PluginDir)
	if err != nil {
		pluginMap.Remove(pluginId)
	}
	return err
}

func InstallPlugin(url string, manifest plugin.Manifest) error {
	//安装前提前关闭进程防止无法操作相关文件
	_ = StopPlugin(manifest.PluginId)
	pluginInfo := initManifest(manifest)
	//已经在下载中不能重复下载
	if pluginInfo.Status == PluginStatusDownloading {
		return nil
	}
	pluginInfo.Status = PluginStatusDownloading
	path, err := httputil2.DownloadFile(url, func(total int64, current int64) {
		pluginInfo.DownloadedSize = current
	})
	if err != nil {
		pluginInfo.Status = PluginStatusDownloadFailed
		pluginInfo.ErrorMsg = err.Error()
		return err
	}
	defer os.Remove(path)
	_ = os.Remove(pluginInfo.PluginDir)
	err = util.Unzip(path, pluginInfo.PluginDir, os.ModePerm)
	if err != nil {
		pluginInfo.Status = PluginStatusInstallationFailed
		//安装失败清除残余文件
		_ = os.Remove(pluginInfo.PluginDir)
		return err
	}
	return nil
}

func run(pluginInfo *PluginInfo) error {
	var cmdStr string
	if runtime.GOOS == "windows" {
		cmdStr = fmt.Sprintf("%s/%s", pluginInfo.PluginDir, pluginInfo.ExecEnter)
	} else {
		cmdStr = fmt.Sprintf("cd %s && ./%s", pluginInfo.PluginDir, pluginInfo.ExecEnter)
	}
	cmd := exec.Command(
		cmdStr,
		"-token", plugin.Token,
		"-namespace", plugin.Namespace,
		"-apiEndpoint", plugin.ApiEndpoint,
		"-masterPort", strconv.FormatInt(int64(plugin.MasterPort), 10),
		"-debug", strconv.FormatBool(plugin.Debug),
	)
	err := cmd.Start()
	pluginInfo.Status = PluginStatusStarting

	go func() {
		defer func() {
			pluginInfo.Status = PluginStatusNotStarted
			err := recover()
			if err != nil {
				logger.Error(err)
			}
		}()
		err = cmd.Wait()
		if err != nil {
			logger.Error(err)
		}
		pluginInfo.Status = PluginStatusNotStarted
	}()

	return err
}

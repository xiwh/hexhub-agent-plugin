package master

import (
	"errors"
	"fmt"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/wonderivan/logger"
	"github.com/xiwh/hexhub-agent-plugin/plugin"
	"github.com/xiwh/hexhub-agent-plugin/util"
	"github.com/xiwh/hexhub-agent-plugin/util/executil"
	httputil2 "github.com/xiwh/hexhub-agent-plugin/util/httputil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type VersionInfo struct {
	Manifest          plugin.Manifest `json:"manifest"`
	PluginId          string          `json:"pluginId"`
	Version           int             `json:"version"`
	VersionName       string          `json:"versionName"`
	UpdateDescription string          `json:"updateDescription"`
	TotalSize         int64           `json:"totalSize"`
	DownloadUrl       string          `json:"downloadUrl"`
}

type CheckUpdateResult struct {
	PluginInfo     *PluginInfo `json:"pluginInfo"`
	Installed      bool        `json:"installed"`
	FirstInstalled bool        `json:"firstInstalled"`
}

type latestPluginInfo struct {
	info VersionInfo
	time int64
}

var latestInfoMap = cmap.New[*latestPluginInfo]()
var pluginMap = cmap.New[*PluginInfo]()
var globalLock = new(sync.Mutex)

func initManifest(manifest plugin.Manifest) *PluginInfo {
	globalLock.Lock()
	defer globalLock.Unlock()
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
			lock:           new(sync.Mutex),
		}
		pluginMap.Set(manifest.PluginId, pluginInfo)
	}
	return pluginInfo
}

func GetPluginInfo(pluginId string) (*PluginInfo, bool) {
	return pluginMap.Get(pluginId)
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
	if ok {
		pluginInfo.lock.Lock()
		defer pluginInfo.lock.Unlock()
		return _stopPlugin(pluginInfo)
	}

	return fmt.Errorf("plugin %s is not running", pluginId)
}

func StartPlugin(pluginId string) error {
	currentInfo, ok := pluginMap.Get(pluginId)
	if ok {
		currentInfo.lock.Lock()
		currentInfo, ok = pluginMap.Get(pluginId)
		if ok {
			//双重检测保证并发调用启动，不会重复启动
			if currentInfo.Status == PluginStatusStarting {
				currentInfo.lock.Unlock()
				return nil
			} else if currentInfo.Status == PluginStatusRunning {
				if Post(pluginId, "ping", nil, nil) == nil {
					currentInfo.lock.Unlock()
					//已启动不用重新启动
					return nil
				} else {
					currentInfo.lock.Unlock()
				}
			} else {
				currentInfo.lock.Unlock()
			}
		} else {
			currentInfo.lock.Unlock()
		}
	}
	//未检测到启动
	manifest, err := plugin.GetManifest(pluginId)
	if err != nil {
		return err
	}
	pluginInfo := initManifest(manifest)
	pluginInfo.lock.Lock()
	defer pluginInfo.lock.Unlock()
	return run(pluginInfo)
}

func CheckUpdate(pluginId string, appVersion int) (result CheckUpdateResult, err error) {
	now := time.Now().UnixMilli()
	latestInfo, _ := latestInfoMap.Get(pluginId)
	if latestInfo == nil || now-latestInfo.time >= 5*60000 {
		info := VersionInfo{}
		//5分钟内只获取一次版本信息
		err = plugin.ApiGet("client/plugin/latest-version", map[string]string{
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"pluginId":   pluginId,
			"appVersion": strconv.Itoa(appVersion),
		}, &info)
		if err != nil {
			return result, err
		}
		if latestInfo == nil {
			latestInfo = &latestPluginInfo{info: info, time: now}
			latestInfoMap.Set(pluginId, latestInfo)
		} else {
			latestInfo.info = info
			latestInfo.time = now
		}
	}

	currentInfo, ok := pluginMap.Get(pluginId)
	result.PluginInfo = currentInfo
	//判断是否已下载并且版本是最新
	if !ok ||
		currentInfo.Status == PluginStatusDownloadFailed ||
		currentInfo.Status == PluginStatusInstallFailed ||
		currentInfo.Version < latestInfo.info.Manifest.Version {

		err = InstallPlugin(latestInfo.info, latestInfo.info.Manifest)
		if err != nil {
			return result, err
		}
		//初次安装完成时，上面会获取不到信息重新获取一次
		currentInfo, ok = pluginMap.Get(pluginId)
		result.PluginInfo = currentInfo
		result.FirstInstalled = !ok
		result.Installed = ok
	}
	if ok {
		//等待其他线程下载完成
		currentInfo.lock.Lock()
		defer currentInfo.lock.Unlock()
	}
	return result, err
}

func UninstallPlugin(pluginId string) error {
	pluginInfo, ok := pluginMap.Get(pluginId)
	if !ok {
		//即便是检测到未安装对应插件也把插件目录删除一下,防止安装过程中发生异常有残留文件跟安装程序发生冲突,导致永远无法重新安装陷入死循环
		_ = os.RemoveAll(filepath.Join(plugin.PluginsDir, pluginId))
		return nil
	}

	pluginInfo.lock.Lock()
	defer pluginInfo.lock.Unlock()

	err := WaitStopPlugin(pluginInfo)
	if err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	err = os.RemoveAll(pluginInfo.PluginDir)
	if err != nil {
		globalLock.Lock()
		defer globalLock.Unlock()
		pluginMap.Remove(pluginId)
	}
	return err
}

func InstallPlugin(latestInfo VersionInfo, manifest plugin.Manifest) error {
	globalLock.Lock()
	//安装前的插件信息
	temp, ok := pluginMap.Get(manifest.PluginId)
	var lastInfo PluginInfo
	if ok {
		lastInfo = *temp
	}
	globalLock.Unlock()

	//安装前提前关闭进程防止无法操作相关文件
	currentInfo := initManifest(manifest)
	currentInfo.lock.Lock()
	defer currentInfo.lock.Unlock()

	err := WaitStopPlugin(currentInfo)
	if err != nil {
		return err
	}

	//已经在下载中不能重复下载
	if currentInfo.Status == PluginStatusDownloading {
		return nil
	}

	//拿到锁后,再次验证版本是否已经为最新避免并发时重复下载安装
	if (ok && lastInfo.Version >= latestInfo.Version) ||
		currentInfo.Status == PluginStatusInstallFailed ||
		currentInfo.Status == PluginStatusDownloadFailed {
		return nil
	}
	currentInfo.Status = PluginStatusDownloading
	currentInfo.TotalSize = latestInfo.TotalSize
	path, err := httputil2.DownloadFile(latestInfo.DownloadUrl, func(total int64, current int64) {
		currentInfo.DownloadedSize = current
	})
	if err != nil {
		currentInfo.Status = PluginStatusDownloadFailed
		currentInfo.ErrorMsg = err.Error()
		return err
	}
	defer os.RemoveAll(path)
	_ = os.RemoveAll(currentInfo.PluginDir)
	err = util.Unzip(path, currentInfo.PluginDir, os.ModePerm)
	if err != nil {
		currentInfo.Status = PluginStatusInstallFailed
		//安装失败清除残余文件
		_ = os.RemoveAll(currentInfo.PluginDir)
		return err
	}
	currentInfo.Status = PluginStatusNotStarted
	return nil
}

func run(pluginInfo *PluginInfo) error {
	//var cmdStr string
	//argsStr := fmt.Sprintf(
	//	"-token=%q -namespace=%q -apiEndpoint=%q -masterPort=%q -debug=%q",
	//	plugin.Token,
	//	plugin.Namespace,
	//	plugin.ApiEndpoint,
	//	strconv.FormatInt(int64(plugin.MasterPort), 10),
	//	strconv.FormatBool(plugin.Debug),
	//)
	//println(argsStr)

	pluginInfo.Status = PluginStatusStarting
	currentEndpoint := pluginInfo.Endpoint

	cmd, err := executil.ExecChildProcess(
		filepath.Join(pluginInfo.PluginDir, pluginInfo.ExecEnter),
		fmt.Sprintf("-token=%s", plugin.Token),
		fmt.Sprintf("-namespace=%s", plugin.Namespace),
		fmt.Sprintf("-apiEndpoint=%s", plugin.ApiEndpoint),
		fmt.Sprintf("-masterPort=%s", strconv.FormatInt(int64(plugin.MasterPort), 10)),
		fmt.Sprintf("-debug=%s", strconv.FormatBool(plugin.Debug)),
	)
	if err != nil {
		pluginInfo.Status = PluginStatusNotStarted
		return err
	}

	go func() {

		defer func() {
			//如果插件进程已经是另外一个进程了，则不用更新状态
			if currentEndpoint == pluginInfo.Endpoint {
				pluginInfo.Status = PluginStatusNotStarted
			}
			err := recover()
			if err != nil {
				logger.Error(err)
			}
		}()
		err = cmd.Wait()
		if err != nil {
			logger.Error(err)
		}
	}()

	return err
}

func WaitStopPlugin(pluginInfo *PluginInfo) error {
	if Post(pluginInfo.Id, "ping", nil, nil) != nil {
		_ = _stopPlugin(pluginInfo)
		//停止成功
		return nil
	}
	for i := 0; i < 25; i++ {
		_ = _stopPlugin(pluginInfo)
		time.Sleep(100 * time.Millisecond)
		if Post(pluginInfo.Id, "ping", nil, nil) != nil {
			//停止成功
			return nil
		}
	}
	return errors.New("stop failed")
}

func _stopPlugin(pluginInfo *PluginInfo) error {
	if pluginInfo.Endpoint != "" {
		err := Post(pluginInfo.Id, "kill", nil, nil)
		pluginInfo.Status = PluginStatusNotStarted
		return err
	}
	return fmt.Errorf("plugin %s is not running", pluginInfo.Id)
}

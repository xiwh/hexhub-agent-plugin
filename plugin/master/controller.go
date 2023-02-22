package master

import (
	"fmt"
	"github.com/wonderivan/logger"
	"github.com/xiwh/hexhub-agent-plugin/plugin"
	httputil2 "github.com/xiwh/hexhub-agent-plugin/util/httputil"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

func checkToken(req *http.Request) bool {
	token := req.Header.Get("Token")
	return token == plugin.Token
}

func pingHandler(writer http.ResponseWriter, req *http.Request) {
	writer.WriteHeader(200)
	_, err := writer.Write([]byte("ok"))
	if err != nil {
		logger.Error(err)
	}
}

func infoHandler(writer http.ResponseWriter, req *http.Request) {
	_ = httputil2.OutResult(writer, httputil2.Success(masterInfo{
		Namespace:   plugin.Namespace,
		Version:     CurrentVersion,
		VersionName: CurrentVersionName,
	}))
}

func checkUpdateHandler(writer http.ResponseWriter, req *http.Request) {
	var latestInfo VersionInfo

	err := plugin.ApiGet("client/plugin/latest-version", map[string]string{
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"pluginId": MasterId,
	}, &latestInfo)
	if err != nil {
		_ = httputil2.OutResult(writer, httputil2.Error(err))
		return
	}

	if CurrentVersion < latestInfo.Version {
		//有更新
		_ = httputil2.OutResult(writer, httputil2.Success(latestInfo))
	}

	_ = httputil2.OutResult(writer, httputil2.Success[any](nil))
}

func pluginListHandler(writer http.ResponseWriter, req *http.Request) {
	keys := pluginMap.Keys()
	values := make([]PluginInfo, len(keys))
	for i := 0; i < len(keys); i++ {
		info, _ := pluginMap.Get(keys[i])
		values[i] = *info
	}
	_ = httputil2.OutResult(writer, httputil2.Success(values))
}

func pluginInfoHandler(writer http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	pluginId := req.Form.Get("pluginId")
	pluginInfo, _ := pluginMap.Get(pluginId)
	if pluginInfo == nil {
		_ = httputil2.OutResult(writer, httputil2.Error(fmt.Errorf("plugin %s does not exist", pluginId)))
	} else {
		_ = httputil2.OutResult(writer, httputil2.Success(pluginInfo))
	}
}

func pluginStartHandler(writer http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	pluginId := req.Form.Get("pluginId")
	err := StartPlugin(pluginId)
	if err != nil {
		_ = httputil2.OutResult(writer, httputil2.Error(err))
		return
	} else {
		//启动之后每个250ms获取一下状态判断是否启动成功,直到尝试超时
		for i := 0; i < 10; i++ {
			time.Sleep(250 * time.Millisecond)
			pluginInfo, ok := pluginMap.Get(pluginId)
			if ok && pluginInfo.Status == PluginStatusRunning {
				_ = httputil2.OutResult(writer, httputil2.Success(""))
				return
			}
		}
	}
	_ = httputil2.OutResult(writer, httputil2.Error(fmt.Errorf("start plugin %s timeout", pluginId)))
}

func pluginRestartHandler(writer http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	pluginId := req.Form.Get("pluginId")
	err := RestartPlugin(pluginId)
	if err != nil {
		_ = httputil2.OutResult(writer, httputil2.Error(err))
		return
	} else {
		//启动之后每个250ms获取一下状态判断是否启动成功,直到尝试超时
		for i := 0; i < 10; i++ {
			time.Sleep(250 * time.Millisecond)
			pluginInfo, ok := pluginMap.Get(pluginId)
			if ok && pluginInfo.Status == PluginStatusRunning {
				_ = httputil2.OutResult(writer, httputil2.Success(""))
				return
			}
		}
	}
	_ = httputil2.OutResult(writer, httputil2.Error(fmt.Errorf("start plugin %s timeout", pluginId)))
}

func pluginStopHandler(writer http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	pluginId := req.Form.Get("pluginId")
	err := StopPlugin(pluginId)
	if err != nil {
		_ = httputil2.OutResult(writer, httputil2.Error(err))
		return
	} else {
		//启动之后每个250ms获取一下状态判断是否关闭成功,直到尝试超时
		for i := 0; i < 10; i++ {
			time.Sleep(250 * time.Millisecond)
			pluginInfo, ok := pluginMap.Get(pluginId)
			if ok && pluginInfo.Status == PluginStatusNotStarted {
				_ = httputil2.OutResult(writer, httputil2.Success(""))
				return
			}
		}
	}
	_ = httputil2.OutResult(writer, httputil2.Error(fmt.Errorf("stop plugin %s timeout", pluginId)))
}

func pluginUninstallHandler(writer http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	pluginId := req.Form.Get("pluginId")
	err := UninstallPlugin(pluginId)
	if err != nil {
		_ = httputil2.OutResult(writer, httputil2.Error(err))
		return
	}
	_ = httputil2.OutResult(writer, httputil2.Success(""))
}

func pluginCheckUpdateHandler(writer http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	pluginId := req.Form.Get("pluginId")
	result, err := CheckUpdate(pluginId)
	if err != nil {
		_ = httputil2.OutResult(writer, httputil2.Error(err))
		return
	}
	_ = httputil2.OutResult(writer, httputil2.Success(result))
}

func pluginRegisterHandler(writer http.ResponseWriter, req *http.Request) {
	//注册插件接口,需要验证token只允许子插件调用,防止第三方程序恶意注册进来
	if !checkToken(req) {
		writer.WriteHeader(401)
		return
	}
	var manifest plugin.Manifest
	err := httputil2.ReadJsonBody(req, &manifest)
	if err != nil {
		logger.Error(err)
		writer.WriteHeader(500)
		return
	}

	oldPluginInfo, ok := pluginMap.Get(manifest.PluginId)
	if ok && oldPluginInfo.Status == PluginStatusRunning {
		//如果当前插件有旧进程在运行则先退出此进程
		err = StopPlugin(manifest.PluginId)
		if err != nil {
			logger.Error(err)
		}
	}

	pluginInfo := initManifest(manifest)
	pluginInfo.Status = PluginStatusRunning
	writer.WriteHeader(200)
	_, err = writer.Write([]byte("ok"))
	if err != nil {
		logger.Error(err)
	}
}

func defaultHandle(writer http.ResponseWriter, req *http.Request) {
	uri := req.URL.RequestURI()
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
		//记录对应插件在线连接数和最后连接时间,用于实现自动退出
		pluginInfo.LastConnTime = time.Now().UnixMilli()
		atomic.AddInt64(&pluginInfo.Connections, 1)
		defer atomic.AddInt64(&pluginInfo.Connections, -1)
		if pluginInfo.Status == PluginStatusRunning && pluginInfo.Endpoint != "" {
			redirectUrl := ""
			if idx != -1 {
				redirectUrl = temp[idx:]
			}
			//通过代理将具体请求转发到对应插件上
			req.Header.Add("Proxy-Url", fmt.Sprintf("http://%s/%s", req.Host, strings.TrimLeft(req.RequestURI, "/")))
			req.URL, _ = url.Parse(pluginInfo.Endpoint)
			req.RequestURI = redirectUrl
			req.Header.Add("Token", plugin.Token)
			mForward.ServeHTTP(writer, req)
			return
		}
	}
	writer.WriteHeader(404)
}
